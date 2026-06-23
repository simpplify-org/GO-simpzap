package main

import (
	"context"
	_ "embed"
	"log"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/simpplify-org/GO-simpzap/app"
)

//go:embed qr.html
var dashHTML []byte

func main() {
	ctx := context.Background()

	//TODO conectar ao banco de dados
	//TODO conecta ao repositorio com a conexao do banco

	svc := app.NewWhatsAppService(ctx) //add adiciona o repositorio do webhook
	h := app.NewWhatsAppHandler(svc)
	h.DashHTML = dashHTML

	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))

	h.RegisterRoutes(e)

	addr := ":8080"
	log.Printf("[MAIN] Servidor iniciado em %s", addr)
	e.Logger.Fatal(e.Start(addr))
}
