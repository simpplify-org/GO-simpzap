package app

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/simpplify-org/GO-simpzap/pkg/token"
	"go.uber.org/zap"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func GetSignatureString() string {
	return os.Getenv("TOKEN_SIGNATURE")
}

func loadMiddlewares(e *echo.Echo) {
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"}, //temp
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
		AllowMethods: middleware.DefaultCORSConfig.AllowMethods,
	}))

	//TODO: mover todos os use para uma func a parte em um arquivo middleware.go dentro da app
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			stop := time.Now()

			e.Logger.Info("request",
				zap.String("method", c.Request().Method),
				zap.String("path", c.Request().URL.Path),
				zap.Int("status", c.Response().Status),
				zap.Duration("duration", stop.Sub(start)),
			)

			return err
		}
	})
}

func checkAuthorization(handlerFunc echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {

		bearerToken := c.QueryParam("token")
		tokenStr := strings.Replace(bearerToken, "Bearer ", "", 1)

		maker, err := token.NewPasetoMaker(GetSignatureString())
		if err != nil {
			log.Println("token.NewPasetoMaker error", zap.Error(err))
			return c.JSON(http.StatusBadGateway, err.Error())
		}

		tokenPayload, err := maker.VerifyToken(tokenStr)

		if err != nil {
			log.Println("token.VerifyToken error", zap.Error(err))
			return c.JSON(http.StatusBadGateway, err.Error())
		}
		c.Set("token_id", tokenPayload.ID)
		c.Set("token_user_id", tokenPayload.UserID)
		c.Set("token_user_nickname", tokenPayload.UserNickname)
		c.Set("token_access_key", tokenPayload.AccessKey)
		c.Set("token_access_ID", tokenPayload.AccessID)
		c.Set("token_tenant_id", tokenPayload.TenantID)
		c.Set("token_expiry_at", tokenPayload.ExpiredAt)
		c.Set("token_user_org_id", tokenPayload.UserOrgId)
		c.Set("token_user_email", tokenPayload.UserEmail)
		c.Set("token_organization_id", tokenPayload.OrganizationID)
		c.Set("token_document", tokenPayload.Document)

		return handlerFunc(c)
	}
}
