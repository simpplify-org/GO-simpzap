package main

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/simpplify-org/GO-simpzap/config"
	"log"
)

func main() {
	loadingEnv := config.NewConfig()
	container := config.NewContainerDI(loadingEnv)

	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	container.Handler.RegisterRoutes(e)

	addr := ":8080"
	log.Printf("[MAIN] Servidor iniciado em %s", addr)
	e.Logger.Fatal(e.Start(addr))
}
