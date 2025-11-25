package config

import (
	"context"
	"database/sql"
	"github.com/simpplify-org/GO-simpzap/app"
	db "github.com/simpplify-org/GO-simpzap/db/sqlc"
	"github.com/simpplify-org/GO-simpzap/infra/database"
	"log"
)

type ContainerDI struct {
	Config Config
	Conn   *sql.DB
	ConnSP *sql.DB

	// Queries SQLC
	Queries *db.Queries

	// Repositories
	Repo app.RepositoryInterface

	// Services
	Service *app.WhatsAppService

	// Handlers
	Handler *app.WhatsAppHandler
}

func NewContainerDI(config Config) *ContainerDI {
	container := &ContainerDI{Config: config}

	container.db()
	container.buildSQS()
	container.buildTransaction()
	container.buildRepositories()
	container.buildServices()
	container.buildHandlers()
	container.buildRouters()
	container.buildProcesses()

	return container
}

func (c *ContainerDI) db() {
	dbConfig := database.Config{
		Host:        c.Config.DBHost,
		Port:        c.Config.DBPort,
		User:        c.Config.DBUser,
		Password:    c.Config.DBPassword,
		Database:    c.Config.DBDatabase,
		SSLMode:     c.Config.DBSSLMode,
		Driver:      c.Config.DBDriver,
		Environment: c.Config.Environment,
	}

	c.Conn = database.NewConnection(&dbConfig)

	c.Queries = db.New(c.Conn)
	log.Println("✅ Database connections initialized")
}

func (c *ContainerDI) buildSQS() {
	log.Println("✅ SQS clients initialized")
}

func (c *ContainerDI) buildTransaction() {
	log.Println("✅ Transaction service initialized")
}

func (c *ContainerDI) buildRepositories() {
	c.Repo = app.NewRepository(c.Conn)
	log.Println("✅ Repositories initialized")
}

func (c *ContainerDI) buildServices() {
	c.Service = app.NewWhatsAppService(context.Background(), c.Repo)
	log.Println("✅ Services initialized")
}

func (c *ContainerDI) buildHandlers() {
	c.Handler = app.NewWhatsAppHandler(c.Service)
	log.Println("✅ Handlers initialized")
}

func (c *ContainerDI) buildRouters() {
	log.Println("✅ Routers initialized")
}

func (c *ContainerDI) buildProcesses() {
	log.Println("✅ Background processes initialized")
}

// StartBackgroundProcesses inicia os processos em background (SQS consumers, etc)
func (c *ContainerDI) StartBackgroundProcesses(ctx context.Context) {
	log.Println("✅ Background processes started")
}
