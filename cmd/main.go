package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/simpplify-org/GO-simpzap/app"
	"log"
	"os"
)

func main() {

	if os.Getenv("TOKEN_SIGNATURE") == "" {
		err := godotenv.Load(".env")
		if err != nil {
			log.Fatal("Error loading .env file")
		}
	}

	os.MkdirAll(".data", 0755)

	client, qrChan, err := app.InitWhatsAppClient()
	if err != nil {
		log.Fatalf("Erro ao iniciar o client: %v", err)
	}

	if qrChan != nil {
		for evt := range qrChan {
			if evt.Event == "code" {
				app.PrintCompactQR(evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	}

	e := echo.New()

	handler := app.NewWhatsAppHandler(client)
	handler.RegisterRoutes(e)

	e.Logger.Fatal(e.Start(":8080"))
}
