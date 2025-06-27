package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"log"
	"os"
	"teste-whatsmeow-simp/internal/app"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	os.MkdirAll(".data", 0755)
	e := echo.New()

	fmt.Println("Listening on :8080")

	app.Endpoints(e)

	e.Logger.Fatal(e.Start(":8080"))
}
