package main

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"log"
	"os"
	"teste-whatsmeow-simp/internal/app"
)

func main() {
	if os.Getenv("TOKEN_SIGNATURE") == "" {
		log.Println("TOKEN_SIGNATURE environment variable not set")
	}

	os.MkdirAll(".data", 0755)
	e := echo.New()

	fmt.Println("Listening on :8080")

	app.Endpoints(e)

	e.Logger.Fatal(e.Start(":8080"))
}
