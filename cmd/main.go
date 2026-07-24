package main

import (
	"context"
	_ "embed"
	"errors"
	"log"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/simpplify-org/GO-simpzap/app"
	"github.com/simpplify-org/GO-simpzap/pkg/authstore"
)

//go:embed qr.html
var dashHTML []byte

//go:embed login.html
var loginHTML []byte

// bootstrapAdmin cria o primeiro usuário admin a partir das variáveis de
// ambiente ADMIN_USER/ADMIN_PASSWORD, caso ainda não exista nenhum usuário.
// Assim cada ambiente (dev/homolog/prod) já sobe com um login pronto, sem
// precisar de um passo manual após o deploy. Usuários extras podem ser
// criados depois via POST /users (autenticado).
func bootstrapAdmin(store *authstore.Store) {
	ctx := context.Background()

	count, err := store.CountUsers(ctx)
	if err != nil {
		log.Fatalf("[AUTH] erro ao checar usuários existentes: %v", err)
	}
	if count > 0 {
		return
	}

	adminUser := os.Getenv("ADMIN_USER")
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminUser == "" || adminPassword == "" {
		log.Println("[AUTH] Nenhum usuário cadastrado e ADMIN_USER/ADMIN_PASSWORD não definidos — /dash ficará inacessível até criar um usuário.")
		return
	}

	if err := store.CreateUser(ctx, adminUser, adminPassword); err != nil && !errors.Is(err, authstore.ErrUserExists) {
		log.Fatalf("[AUTH] erro ao criar usuário admin inicial: %v", err)
	}
	log.Printf("[AUTH] Usuário admin '%s' criado a partir do .env", adminUser)
}

func main() {
	ctx := context.Background()

	//TODO conectar ao banco de dados
	//TODO conecta ao repositorio com a conexao do banco

	if err := os.MkdirAll(".data", 0755); err != nil {
		log.Fatalf("[MAIN] erro ao criar diretório .data: %v", err)
	}

	authDBPath := os.Getenv("AUTH_DB_PATH")
	if authDBPath == "" {
		authDBPath = ".data/master.db"
	}
	authStore, err := authstore.New(authDBPath)
	if err != nil {
		log.Fatalf("[AUTH] erro ao abrir banco de autenticação: %v", err)
	}
	bootstrapAdmin(authStore)

	svc := app.NewWhatsAppService(ctx) //add adiciona o repositorio do webhook
	authHandler := app.NewAuthHandler(authStore)
	h := app.NewWhatsAppHandler(svc, authHandler)
	h.DashHTML = dashHTML
	h.LoginHTML = loginHTML

	// DEVICE_API_KEY protege /create, /devices, /delete e /device/* com o
	// header X-API-Key. Fica desligado (rotas abertas, igual hoje) até essa
	// variável ser definida — dá pra atualizar os serviços consumidores com
	// o header antes de ativar a exigência no master, sem quebrar nada no
	// meio do caminho.
	deviceAPIKey := os.Getenv("DEVICE_API_KEY")
	if deviceAPIKey == "" {
		log.Println("[MAIN] ⚠️  DEVICE_API_KEY não definida — /create, /devices, /delete e /device/* continuam sem autenticação.")
	} else {
		log.Println("[MAIN] 🔒 DEVICE_API_KEY definida — /create, /devices, /delete e /device/* agora exigem o header X-API-Key.")
	}
	h.APIKeyMiddleware = app.NewAPIKeyMiddleware(deviceAPIKey)

	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, "X-API-Key"},
	}))

	h.RegisterRoutes(e)

	addr := ":8080"
	log.Printf("[MAIN] Servidor iniciado em %s", addr)
	e.Logger.Fatal(e.Start(addr))
}
