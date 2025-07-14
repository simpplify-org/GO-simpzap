package app

import "github.com/labstack/echo/v4"

func Endpoints(e *echo.Echo) {
	ws := e.Group("/ws", checkAuthorization)
	ws.GET("/whatsapp", func(c echo.Context) error {
		WsHandler(c)
		return nil
	})
}
