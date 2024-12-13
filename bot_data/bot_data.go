package bot_data

import (
	"QADots/database"
	"bytes"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"
)

const (
	tasksListPrint = `Команды для работы с ботом: 
/start - зарегистрироваться
/ask <вопрос>~<tags> - задать вопрос.
/answer <номер вопроса>~<ответ> - ответить на вопрос. 
/get_answers <номер вопроса> - получить все текущие ответы на вопрос 
/questions <тег> - получить 10 самых залайканных вопросов по тегу 
/my_questions - получить все заданные Вами вопросы
/like_question <номер вопроса> - поставить лайк вопросу
/like_answer <номер ответа> - поставить лайк ответу
/help - показать все возможные команды`
)

var (
	BotToken   = flag.String("tg.token", "", "token for telegram")
	WebhookURL = flag.String("tg.webhook", "", "webhook addr for telegram")
)

type Bot struct {
	API    *tgbotapi.BotAPI
	dtbase *database.DB
}

func (b *Bot) Init() error {
	bot, err := tgbotapi.NewBotAPI(*BotToken)
	if err != nil {
		return err
	}

	b.dtbase = database.InitDB()

	b.API = bot

	b.API.Debug = true

	log.Printf("Authorized on account %s", b.API.Self.UserName)

	// Полный URL для Webhook, включая токе
	webh, err := tgbotapi.NewWebhook(*WebhookURL)
	if err != nil {
		return fmt.Errorf("ошибка создания webhook: %v", err)
	}

	// Устанавливаем Webhook
	_, err = b.API.Request(webh)
	if err != nil {
		log.Printf("ошибка установки webhook: %v", err)
	}

	return nil
}

type Task struct {
	ID      int64
	Title   string
	Owner   *tgbotapi.User
	ForWhom *tgbotapi.User
}

func (b *Bot) Help() string {
	return tasksListPrint
}

func (b *Bot) Start(u *tgbotapi.User) string {
	query := `
	INSERT INTO public.users (
		user_id, username, registration_date, question_count, answer_count, status_id
	) VALUES ($1, $2, NOW(), $3, $4, $5)
	ON CONFLICT (user_id) DO NOTHING
	RETURNING user_id
	`

	userID := 0
	b.dtbase.Db.QueryRow(query, u.ID, u.UserName, 0, 0, 1).Scan(&userID)

	if userID == 0 {
		return "Пользователь уже существует"
	}

	fmt.Printf("Новый пользователь добавлен с ID %d\n", userID)
	return "Успешная регистрация"
}

func (b *Bot) linkTagQuestion(tag_id int, question int) {
	query := `
	INSERT INTO public.questiontags(
	question_id, tag_id)
	VALUES ($1, $2)
	ON CONFLICT DO NOTHING;
	`
	_, err := b.dtbase.Db.Exec(query, question, tag_id)
	if err != nil {
		log.Printf("Ошибка при линковке тега и question_id: %v", err)
	}
	fmt.Printf("Линковка тега и question_id прошла успешно: %d \n", tag_id)
}

func (b *Bot) AddTags(tags []string, question_id int) {
	query := `
	INSERT INTO public.tags(tag_name)
	VALUES ($1)
	ON CONFLICT (tag_name) DO UPDATE
    SET tag_name = EXCLUDED.tag_name
	RETURNING tag_id;
`

	tag_id := 0
	for _, tag := range tags {
		err := b.dtbase.Db.QueryRow(query, tag).Scan(&tag_id)
		if err != nil {
			log.Printf("Ошибка при добавлении тега: %v", err)
		}
		fmt.Printf("Новый тег добавлен: %s \n", tag)
		b.linkTagQuestion(tag_id, question_id)
	}
}

func (b *Bot) checkRegistration(userID int64) bool {
	query := "SELECT CheckUserRegistration($1);"

	var exist bool
	_ = b.dtbase.Db.QueryRow(query, userID).Scan(&exist)
	return exist
}

func (b *Bot) isQuestionExist(userID int64) bool {
	query := "SELECT CheckQuestionExistence($1);"

	var exist bool
	_ = b.dtbase.Db.QueryRow(query, userID).Scan(&exist)
	return exist
}

func (b *Bot) isAnswerExist(userID int64) bool {
	query := "SELECT CheckAnswerExistence($1);"

	var exist bool
	_ = b.dtbase.Db.QueryRow(query, userID).Scan(&exist)
	return exist
}

func parseArgs(args string) (string, []string, error) {
	res := strings.Split(args, "~")
	if len(res) != 2 {
		return "", nil, errors.New("Неправильно переданы аргументы")
	}
	return res[0], strings.Split(res[1], " "), nil
}

func (b *Bot) Ask(u *tgbotapi.User, args string) string {

	exist := b.checkRegistration(u.ID)
	if !exist {
		return "Сначала зарегистрируйтесь с помощью команды /start"
	}

	question, tags, err := parseArgs(args)
	if err != nil {
		return err.Error()
	}
	query := `
		INSERT INTO public.questions(
		user_id, question_text, created_at, is_closed)
		VALUES ($1, $2, $3, $4)
		RETURNING question_id;
	`
	question_id := 0
	err = b.dtbase.Db.QueryRow(query, u.ID, question, "NOW()", 0).Scan(&question_id)
	if err != nil {
		log.Printf("Ошибка при добавлении вопроса, повторите ещё раз: %v", err)
	}

	b.AddTags(tags, question_id)

	fmt.Printf("Новый вопрос\n")
	return "Вопрос добавлен успешно. Ожидайте ответа от пользователей"
}

func (b *Bot) Questions(u *tgbotapi.User, arg string) string {
	query := `
		SELECT u.username, q.question_text, q.created_at, q.question_id, COUNT(ql.like_id) AS like_count
		FROM public.questions q
		JOIN public.questiontags qt ON q.question_id = qt.question_id
		JOIN public.tags t ON qt.tag_id = t.tag_id
		JOIN public.users u ON q.user_id = u.user_id
		LEFT JOIN public.questionlikes ql ON q.question_id = ql.question_id
		WHERE t.tag_name = $1 AND q.is_closed = $2
		GROUP BY u.username, q.question_text, q.created_at, q.question_id
		ORDER BY like_count DESC
		LIMIT 10;
	`
	rows, err := b.dtbase.Db.Query(query, arg, false)
	if err != nil {
		log.Printf("Ошибка при поиске вопросов по данному тегу: %v", err)
	}
	defer rows.Close()

	var csvData [][]string
	csvData = append(csvData, []string{"Username", "Question Text", "Created At", "Question ID", "Like Count"})

	count := 0
	var result string
	for rows.Next() {
		var question struct {
			username      string
			QuestionText  string
			CreatedAt     string
			questionNum   int64
			questionLikes int
		}

		err := rows.Scan(&question.username, &question.QuestionText, &question.CreatedAt, &question.questionNum, &question.questionLikes)
		result += "Вопрос от пользователя " + question.username +
			" создан " + question.CreatedAt +
			" номер вопроса " + strconv.FormatInt(question.questionNum, 10) +
			"\n" + question.QuestionText + "\n" + "Количество лайков: " + strconv.Itoa(question.questionLikes) + "\n\n"

		if err != nil {
			log.Printf("Ошибка при сканировании строки: %v", err)
		}
		count++
		csvData = append(csvData, []string{
			question.username,
			question.QuestionText,
			question.CreatedAt,
			strconv.FormatInt(question.questionNum, 10),
			strconv.Itoa(question.questionLikes),
		})
	}

	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)

	for _, record := range csvData {
		if err := writer.Write(record); err != nil {
			log.Printf("Ошибка при записи в CSV буфер: %v", err)
		}
	}
	writer.Flush()

	doc := tgbotapi.NewDocument(int64(u.ID), tgbotapi.FileBytes{
		Name:  "questions.csv",
		Bytes: buffer.Bytes(),
	})

	if _, err := b.API.Send(doc); err != nil {
		log.Printf("Ошибка при отправке файла: %v", err)
		return "Ошибка при отправке файла."
	}

	if count == 0 {
		return "Не найдено ни одного вопроса по тегу"
	} else {
		return result
	}
}

func (b *Bot) Like_Question(u *tgbotapi.User, arg string) string {
	parseArg, _ := strconv.ParseInt(arg, 10, 64)
	exist := b.isQuestionExist(parseArg)
	if !exist {
		return "Такого вопроса не существует"
	}
	exist = b.checkRegistration(u.ID)
	if !exist {
		return "Сначала зарегистрируйтесь с помощью команды /start"
	}
	query := `
		INSERT INTO QuestionLikes (question_id, user_id)
		SELECT $1, $2
		WHERE EXISTS (
			SELECT 1
			FROM Questions
			WHERE question_id = $1
		)
		AND NOT EXISTS (
			SELECT 1
			FROM QuestionLikes
			WHERE question_id = $1 AND user_id = $2
		)
		ON CONFLICT DO NOTHING
		RETURNING question_id;
	`
	q_id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return "Ошибка в введении номера вопроса"
	}
	exist_q_id := 0
	b.dtbase.Db.QueryRow(query, q_id, u.ID).Scan(&exist_q_id)
	if exist_q_id == 0 {
		return "Вы уже ставили лайк этому вопросу"
	}

	return "Лайк добавлен успешно."
}

func (b *Bot) My_Questions(u *tgbotapi.User) string {
	exist := b.checkRegistration(u.ID)
	if !exist {
		return "Сначала зарегистрируйтесь с помощью команды /start"
	}
	query := `
		SELECT q.question_id, q.question_text, q.created_at, COUNT(ql.like_id) AS like_count
		FROM public.questions q
		LEFT JOIN public.questionlikes ql ON q.question_id = ql.question_id
		WHERE q.user_id = $1
		GROUP BY q.question_id, q.question_text, q.created_at, q.is_closed;
	`
	rows, err := b.dtbase.Db.Query(query, u.ID)
	if err != nil {
		log.Printf("Ошибка при поиске ваших вопросов: %v", err)
	}
	defer rows.Close()

	var csvData [][]string
	csvData = append(csvData, []string{"QuestionText", "CreatedAt", "questionNum", "questionLikes"})
	count := 0
	var result string
	for rows.Next() {
		var question struct {
			QuestionText  string
			CreatedAt     string
			questionNum   int64
			questionLikes int
		}

		err := rows.Scan(&question.questionNum, &question.QuestionText, &question.CreatedAt, &question.questionLikes)
		result += "Вопрос от пользователя " + u.UserName +
			" создан " + question.CreatedAt +
			" номер вопроса " + strconv.FormatInt(question.questionNum, 10) +
			"\n" + question.QuestionText + "\n" + "Количество лайков: " + strconv.Itoa(question.questionLikes) + "\n\n"

		if err != nil {
			log.Printf("Ошибка при сканировании строки: %v", err)
		}
		count++
		csvData = append(csvData, []string{
			question.QuestionText,
			question.CreatedAt,
			strconv.FormatInt(question.questionNum, 10),
			strconv.Itoa(question.questionLikes),
		})

	}

	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)

	for _, record := range csvData {
		if err := writer.Write(record); err != nil {
			log.Printf("Ошибка при записи в CSV буфер: %v", err)
		}
	}
	writer.Flush()

	doc := tgbotapi.NewDocument(int64(u.ID), tgbotapi.FileBytes{
		Name:  "questions.csv",
		Bytes: buffer.Bytes(),
	})

	if _, err := b.API.Send(doc); err != nil {
		log.Printf("Ошибка при отправке файла: %v", err)
		return "Ошибка при отправке файла."
	}

	if count == 0 {
		return "Не найдено ни одного вопроса"
	} else {
		return result
	}
}

func (b *Bot) Answer(u *tgbotapi.User, args []string) string {
	parseArg, _ := strconv.ParseInt(args[0], 10, 64)
	exist := b.isQuestionExist(parseArg)
	if !exist {
		return "Такого вопроса не существует"
	}
	exist = b.checkRegistration(u.ID)
	if !exist {
		return "Сначала зарегистрируйтесь с помощью команды /start"
	}
	query := `
		INSERT INTO Answers (question_id, user_id, answer_text)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING;
	`
	_, err := b.dtbase.Db.Query(query, args[0], u.ID, args[1])
	if err != nil {
		log.Printf("Ошибка при добавлении ответа, повторите ещё раз: %v", err)
	}

	return "Ответ добавлен успешно. Ожидайте лайков)"
}

func (b *Bot) Get_Answers(u *tgbotapi.User, arg string) string {
	parseArg, _ := strconv.ParseInt(arg, 10, 64)
	exist := b.isQuestionExist(parseArg)
	if !exist {
		return "Такого вопроса не существует"
	}
	query := `
		SELECT a.answer_id, a.answer_text, u.username, s.status_name, a.created_at AS answer_time, COUNT(al.like_id) AS like_count
		FROM Answers a
		JOIN Users u ON a.user_id = u.user_id
		JOIN Statuses s ON u.status_id = s.status_id
		LEFT JOIN AnswerLikes al ON a.answer_id = al.answer_id
		WHERE a.question_id = $1
		GROUP BY a.answer_id, a.answer_text, u.username, s.status_name, a.created_at;
	`
	q_id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return "Ошибка в введении номера вопроса"
	}
	rows, err1 := b.dtbase.Db.Query(query, q_id)
	if err1 != nil {
		return "Ошибка. Попробуйте еще раз."
	}
	defer rows.Close()

	var csvData [][]string
	csvData = append(csvData, []string{"answerID", "answerText", "username", "statusName", "created", "likeCount"})

	count := 0
	var result string
	for rows.Next() {
		var answer struct {
			answerID   int64
			answerText string
			username   string
			statusName string
			created    string
			likeCount  int
		}

		err := rows.Scan(&answer.answerID, &answer.answerText, &answer.username, &answer.statusName, &answer.created, &answer.likeCount)
		result += "Ответ от пользователя " + answer.username +
			" создан " + answer.created +
			" номер ответа " + strconv.FormatInt(answer.answerID, 10) +
			"\n" + answer.answerText + "\n" + "Количество лайков: " + strconv.Itoa(answer.likeCount) + "\n" +
			"Статус пользователя: " + answer.statusName + "\n\n"

		if err != nil {
			log.Printf("Ошибка при сканировании строки: %v", err)
		}
		count++
		csvData = append(csvData, []string{
			strconv.FormatInt(answer.answerID, 10),
			answer.answerText,
			answer.username,
			answer.statusName,
			answer.created,
			strconv.Itoa(answer.likeCount),
		})
	}

	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)

	for _, record := range csvData {
		if err := writer.Write(record); err != nil {
			log.Printf("Ошибка при записи в CSV буфер: %v", err)
		}
	}
	writer.Flush()

	doc := tgbotapi.NewDocument(int64(u.ID), tgbotapi.FileBytes{
		Name:  "questions.csv",
		Bytes: buffer.Bytes(),
	})

	if _, err := b.API.Send(doc); err != nil {
		log.Printf("Ошибка при отправке файла: %v", err)
		return "Ошибка при отправке файла."
	}

	if count == 0 {
		return "Не найдено ни одного вопроса"
	} else {
		return result
	}
}

func (b *Bot) Like_Answer(u *tgbotapi.User, arg string) string {
	parseArg, _ := strconv.ParseInt(arg, 10, 64)
	exist := b.isAnswerExist(parseArg)
	if !exist {
		return "Такого ответа не существует"
	}
	exist = b.checkRegistration(u.ID)
	if !exist {
		return "Сначала зарегистрируйтесь с помощью команды /start"
	}
	query := `
		INSERT INTO AnswerLikes (answer_id, user_id)
		SELECT $1, $2
		WHERE EXISTS (
			SELECT 1
			FROM Answers
			WHERE answer_id = $1
		)
		AND NOT EXISTS (
			SELECT 1
			FROM AnswerLikes
			WHERE answer_id = $1 AND user_id = $2
		)
		ON CONFLICT DO NOTHING
		RETURNING answer_id;
	`
	a_id, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return "Ошибка в введении номера ответа"
	}

	exist_a_id := 0
	b.dtbase.Db.QueryRow(query, a_id, u.ID).Scan(&exist_a_id)
	if exist_a_id == 0 {
		return "Вы уже ставили лайк этому ответу"
	}

	return "Лайк добавлен успешно."
}
