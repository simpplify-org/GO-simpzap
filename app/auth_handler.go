package app

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/simpplify-org/GO-simpzap/pkg/authstore"
)

const sessionCookieName = "simpzap_session"

type AuthHandler struct {
	Store *authstore.Store
}

func NewAuthHandler(store *authstore.Store) *AuthHandler {
	return &AuthHandler{Store: store}
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil || req.Username == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "informe usuário e senha"})
	}

	user, err := h.Store.Authenticate(c.Request().Context(), req.Username, req.Password)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "usuário ou senha inválidos"})
	}

	token, expiresAt, err := h.Store.CreateSession(c.Request().Context(), user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "erro ao criar sessão"})
	}

	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Request().TLS != nil,
	})

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "username": user.Username})
}

func (h *AuthHandler) Logout(c echo.Context) error {
	if cookie, err := c.Cookie(sessionCookieName); err == nil {
		_ = h.Store.DeleteSession(c.Request().Context(), cookie.Value)
	}
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	return c.JSON(http.StatusOK, map[string]string{"status": "logged_out"})
}

// CreateUser cadastra um novo admin. Só pode ser chamado por quem já está
// logado (protegido por RequireAuth) — não existe cadastro público.
func (h *AuthHandler) CreateUser(c echo.Context) error {
	var req CreateUserRequest
	if err := c.Bind(&req); err != nil || req.Username == "" || req.Password == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "informe usuário e senha"})
	}
	if len(req.Password) < 8 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "senha deve ter ao menos 8 caracteres"})
	}

	if err := h.Store.CreateUser(c.Request().Context(), req.Username, req.Password); err != nil {
		if errors.Is(err, authstore.ErrUserExists) {
			return c.JSON(http.StatusConflict, map[string]string{"error": "usuário já existe"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "erro ao criar usuário"})
	}

	return c.JSON(http.StatusCreated, map[string]string{"status": "created", "username": req.Username})
}

// RequireAuth protege rotas exigindo um cookie de sessão válido.
func (h *AuthHandler) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		cookie, err := c.Cookie(sessionCookieName)
		if err != nil {
			return c.Redirect(http.StatusFound, "/login")
		}

		user, err := h.Store.ValidateSession(c.Request().Context(), cookie.Value)
		if err != nil {
			return c.Redirect(http.StatusFound, "/login")
		}

		c.Set("user", user)
		return next(c)
	}
}
