# include
include .env

# Swagger
swag:
	@echo "$(YELLOW) Generating $(CYAN) Swagger $(GREEN). $(NC)"
	swag init -g cmd/api.go --output docs/app

# Local Setup
postgres-setup:
	docker run --name login-postgres -p ${DB_PORT}:${DB_PORT}/tcp -e DB_USER=${DB_USER} -e DB_PASSWORD=${DB_PASSWORD} -d postgres:12-alpine

start-postgres:
	docker start login-postgres

stop-postgres:
	docker stop login-postgres

createdb: 
	docker exec -it login-postgres createdb --username=${DB_USER} --owner=${DB_USER} $(DB_DATABASE)

# local migrations
migration-up:
	@echo "$(CYAN)Starting local migration... $(GREEN) UP $(NC)"
	migrate -path db/migration -database "postgresql://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_DATABASE}?sslmode=disable" -verbose up

migration-down:
	@echo "$(CYAN)Starting local migration... $(RED) DOWN $(NC)"
	migrate -path db/migration -database "postgresql://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_DATABASE}?sslmode=disable" -verbose down

# run locally
run: 
	@echo "$(GREEN) Running Golang App... $(CYAN)LOCAL $(NC)"
	go run cmd/main.go

# internal commands
sqlc:
	@echo "$(YELLOW) Generating $(CYAN) sqlc $(YELLOW)files... $(NC)"
	sqlc generate

nodemon:
	nodemon --exec go run main.go --signal SIGTERM
