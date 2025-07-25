FROM golang:1.24.4-alpine AS builder

WORKDIR /app

# Instala ferramentas necessárias para compilar com CGO + SQLite
RUN apk add --no-cache gcc musl-dev sqlite-dev

COPY .env .env

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# ATENÇÃO: precisa do CGO_ENABLED=1
RUN CGO_ENABLED=1 go build -o main ./cmd/main.go

# Etapa final usando Alpine com certificados
FROM alpine

WORKDIR /app

RUN apk add --no-cache ca-certificates sqlite-libs

RUN mkdir /app/.data && chmod 755 /app/.data

COPY --from=builder /app/main .

EXPOSE 8080

CMD ["./main"]
