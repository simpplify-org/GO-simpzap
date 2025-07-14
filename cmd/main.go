package main

import (
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/simpplify-org/GO-simpzap/app"
	"log"
	"os"
)

func main() {
	//err := godotenv.Load(".env")
	//if err != nil {
	//	log.Fatal("Error loading .env file")
	//}

	if os.Getenv("TOKEN_SIGNATURE") == "" {
		log.Println("TOKEN_SIGNATURE environment variable not set")
	}

	os.MkdirAll(".data", 0755)
	e := echo.New()

	fmt.Println("Listening on :8080")

	app.Endpoints(e)

	e.Logger.Fatal(e.Start(":8080"))
}
