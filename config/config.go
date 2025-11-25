package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerName  string
	ServerPort  string
	Environment string
	DBHost      string
	DBPort      string
	DBUser      string
	DBPassword  string
	DBDatabase  string
	DBSSLMode   string
	DBDriver    string
}

func NewConfig() Config {
	if os.Getenv("ENVIRONMENT") == "" {
		if err := godotenv.Load(".env"); err != nil {
			panic("Error loading env file")
		}
	}
	return Config{
		ServerName:  os.Getenv("SERVER_NAME"),
		ServerPort:  os.Getenv("SERVER_PORT"),
		Environment: os.Getenv("ENVIRONMENT"),
		DBHost:      os.Getenv("POSTGRES_HOST"),
		DBPort:      os.Getenv("POSTGRES_PORT"),
		DBUser:      os.Getenv("POSTGRES_USER"),
		DBPassword:  os.Getenv("POSTGRES_PASSWORD"),
		DBDatabase:  os.Getenv("POSTGRES_DB"),
		DBSSLMode:   os.Getenv("DB_SSL_MODE"),
		DBDriver:    os.Getenv("DB_DRIVER"),
	}
}
