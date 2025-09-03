package main

import (
	"context"
	"log"
	"os"

	"github.com/labstack/echo/v4/middleware"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/simpplify-org/GO-data-connector-lib/slack"
	"github.com/simpplify-org/GO-simpzap/app"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	if os.Getenv("MONGO_URI") == "" {
		if err := godotenv.Load(".env"); err != nil {
			panic("Error loading env file")
		}
	}

	mongoClient, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(os.Getenv("MONGO_URI")))
	if err != nil {
		log.Fatal("Erro ao conectar ao MongoDB: ", err)
	}

	db := mongoClient.Database("simpzap")

	config := slack.Config{
		SlackToken:        os.Getenv("SLACK_TOKEN"),
		ChannelID:         os.Getenv("SLACK_CHANNEL"),
		CriticalChannelID: os.Getenv("SLACK_CRITICAL_CHANNEL"),
		OnlyPanics:        false,
		Debug:             false,
		Timeout:           0,
	}
	reporter := slack.New(config)

	deviceRepo := app.NewDeviceRepository(db)
	messageRepo := app.NewMessageHistoryRepository(db)
	listContactRepo := app.NewContactListRepository(db)
	waService := app.NewWhatsAppService(deviceRepo, messageRepo, listContactRepo)
	waHandler := app.NewWhatsAppHandler(waService, reporter)

	e := echo.New()

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"}, // ou "*"
		AllowMethods: []string{echo.GET, echo.POST, echo.OPTIONS},
	}))

	waHandler.RegisterRoutes(e)
	e.Logger.Fatal(e.Start(":8080"))
}
