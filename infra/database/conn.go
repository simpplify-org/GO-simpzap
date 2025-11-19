package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

type Config struct {
	Host        string
	Port        string
	User        string
	Password    string
	Database    string
	SSLMode     string
	Driver      string
	Environment string
}

func NewConnection(config *Config) *sql.DB {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s application_name=SIMPZAP-%s",
		config.Host, config.Port, config.User, config.Password, config.Database, config.SSLMode, config.Environment,
	)

	db, err := sql.Open(config.Driver, dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	log.Println("Database connection established successfully")
	return db
}
