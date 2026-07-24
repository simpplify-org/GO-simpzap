package app

import (
	"crypto/subtle"
	"net/http"

	"github.com/labstack/echo/v4"
)

// NewAPIKeyMiddleware protege as rotas de gerenciamento/proxy de device
// (/create, /devices, /delete, /device/*) exigindo o header "X-API-Key".
//
// Se key estiver vazia, devolve um middleware no-op — mantém as rotas
// abertas como hoje, sem quebrar integrações existentes. O time liga a
// checagem quando quiser, configurando DEVICE_API_KEY no master e o mesmo
// valor no header X-API-Key de cada serviço consumidor. A forma de chamar
// não muda: continua /device/<numero>/..., sem porta fixa.
func NewAPIKeyMiddleware(key string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		if key == "" {
			return next
		}
		return func(c echo.Context) error {
			got := c.Request().Header.Get("X-API-Key")
			if len(got) != len(key) || subtle.ConstantTimeCompare([]byte(got), []byte(key)) != 1 {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "X-API-Key inválida ou ausente"})
			}
			return next(c)
		}
	}
}
