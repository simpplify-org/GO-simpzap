package main

import (
	"context"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/simpplify-org/GO-simpzap/app"
	"log"
)

func main() {
	ctx := context.Background()

	svc := app.NewWhatsAppService(ctx)
	h := app.NewWhatsAppHandler(svc)

	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	h.RegisterRoutes(e)

	addr := ":8080"
	log.Printf("[MAIN] Servidor iniciado em %s", addr)
	e.Logger.Fatal(e.Start(addr))
}
