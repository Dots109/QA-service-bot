package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
)

type DBConfig struct {
	Database struct {
		User     string `json:"user"`
		Password string `json:"password"`
		Dbname   string `json:"dbname"`
		Sslmode  string `json:"sslmode"`
		Port     string `json:"Port"`
	} `json:"database"`
}

func loadConfig(configPath string) (*DBConfig, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("ошибка при открытии конфигурационного файла: %v", err)
	}
	defer file.Close()

	var config DBConfig
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		return nil, fmt.Errorf("ошибка при парсинге конфигурации: %v", err)
	}

	return &config, nil
}

type DB struct {
	Db *sql.DB
}

func InitDB() *DB {
	var dtbase DB
	var err error
	config, err := loadConfig("../configs/db_config.json")
	if err != nil {
		log.Fatalf("Config was not load: %v", err)
	}

	dsl := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=%s port=%s",
		config.Database.User, config.Database.Password, config.Database.Dbname, config.Database.Sslmode, config.Database.Port)
	fmt.Printf(dsl)
	dtbase.Db, err = sql.Open("postgres", dsl)
	if err != nil {
		log.Fatal("Ошибка подключения к базе данных: ", err)
	}

	err = dtbase.Db.Ping()
	if err != nil {
		log.Fatal("Ошибка при проверке подключения к базе данных: ", err)
	}

	fmt.Println("Подключение к базе данных установлено успешно!")
	return &dtbase
}
