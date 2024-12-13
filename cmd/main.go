package main

import (
	"QADots/bot_data"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tgbotapi "github.com/skinass/telegram-bot-api/v5"
)

var (
	helperKeyboard = tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("/help"),
			tgbotapi.NewKeyboardButton("/ask"),
			tgbotapi.NewKeyboardButton("/choose"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("/answer"),
			tgbotapi.NewKeyboardButton("/questions"),
			tgbotapi.NewKeyboardButton("/my_questions"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("/like_question"),
			tgbotapi.NewKeyboardButton("/like_answer"),
			tgbotapi.NewKeyboardButton("/get_answers"),
		),
	)

	Port = ":8080"
)

func ParseMessageCommand(cmd string, sent string) ([]string, error) {
	stringList := strings.Split(cmd, sent)
	return stringList, nil
}

func checkArguments(args []string, count int) error {
	if len(args) < count {
		return errors.New("Введены неправильные аргументы. Посмотрите help")
	}
	return nil
}

// startTaskBot запускает сервер и слушает вебхуки Telegram
func startTaskBot(ctx context.Context) error {
	var b bot_data.Bot
	err := b.Init()
	if err != nil {
		log.Fatalf("Bot can't init: %v", err)
	}

	// Обработка запросов от Telegram через Webhook
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		update, err := b.API.HandleUpdate(r)
		if err != nil {
			http.Error(w, "Error handling update", http.StatusBadRequest)
			return
		}

		if update.Message == nil {
			return
		}

		var msg tgbotapi.MessageConfig

		cmd, err := ParseMessageCommand(update.Message.Command(), " ")
		if err != nil {
			msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Неверный формат ID")
			_, err = b.API.Send(msg)
			if err != nil {
				if err = json.NewEncoder(w).Encode(err); err != nil {
					return
				}
			}
			return
		}

		log.Printf("start task bot")
		switch cmd[0] {
		case "help":
			log.Printf("print help")
			msg = tgbotapi.NewMessage(update.Message.Chat.ID, b.Help())
			msg.ReplyMarkup = helperKeyboard

		case "start":
			msg = tgbotapi.NewMessage(update.Message.Chat.ID, b.Start(update.Message.From))

		case "ask":
			msg = tgbotapi.NewMessage(update.Message.Chat.ID, b.Ask(update.Message.From, update.Message.CommandArguments()))

		case "answer":
			args, _ := ParseMessageCommand(update.Message.CommandArguments(), "~")
			if checkArguments(args, 1) == nil {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, b.Answer(update.Message.From, args))
			} else {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка при передаче аргументов")
			}

		case "questions":
			args, _ := ParseMessageCommand(update.Message.CommandArguments(), " ")
			if checkArguments(args, 1) == nil {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, b.Questions(update.Message.From, args[0]))
			} else {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка при передаче аргументов")
			}

		case "my_questions":
			msg = tgbotapi.NewMessage(update.Message.Chat.ID, b.My_Questions(update.Message.From))

		case "like_question":
			args, _ := ParseMessageCommand(update.Message.CommandArguments(), " ")
			if checkArguments(args, 1) == nil {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, b.Like_Question(update.Message.From, args[0]))
			} else {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка при передаче аргументов")
			}

		case "like_answer":
			args, _ := ParseMessageCommand(update.Message.CommandArguments(), " ")
			if checkArguments(args, 1) == nil {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, b.Like_Answer(update.Message.From, args[0]))
			} else {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка при передаче аргументов")
			}

		case "get_answers":
			args, _ := ParseMessageCommand(update.Message.CommandArguments(), " ")
			if checkArguments(args, 1) == nil {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, b.Get_Answers(update.Message.From, args[0]))
			} else {
				msg = tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка при передаче аргументов")
			}

		default:
			msg = tgbotapi.NewMessage(update.Message.Chat.ID, "я не знаю такой команды :(")
		}

		_, err = b.API.Send(msg)
		if err != nil {
			if err = json.NewEncoder(w).Encode(err); err != nil {
				return
			}
		}
	})

	// Создаем http.Server
	srv := &http.Server{Addr: Port}

	// Запуск сервера в отдельной горутине
	go func() {
		log.Println("Запуск сервера на порту", Port)
		err := srv.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatalf("Ошибка при запуске сервера: %v", err)
		}
	}()

	// Ожидание завершения контекста и остановка сервера
	<-ctx.Done()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("ошибка завершения работы сервера: %v", err)
	}

	return nil
}

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Запуск бота в отдельной горутине
	go func() {
		if err := startTaskBot(ctx); err != nil {
			log.Fatalf("Ошибка при запуске бота: %v", err)
		}
	}()

	// Ожидание сигнала завершения
	<-sigChan
	log.Println("Получен сигнал завершения, завершаем работу...")
	cancel()
}
