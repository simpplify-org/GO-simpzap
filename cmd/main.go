package main

import (
	"context"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/simpplify-org/GO-simpzap/app"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"os"
)

func main() {
	_ = godotenv.Load(".env")

	mongoClient, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(os.Getenv("MONGO_URI")))
	if err != nil {
		log.Fatal("Erro ao conectar ao MongoDB: ", err)
	}
	db := mongoClient.Database("simpzap")

	deviceRepo := app.NewDeviceRepository(db)
	waService := app.NewWhatsAppService(deviceRepo)
	waHandler := app.NewWhatsAppHandler(waService)

	e := echo.New()
	waHandler.RegisterRoutes(e)
	e.Logger.Fatal(e.Start(":8080"))
}
