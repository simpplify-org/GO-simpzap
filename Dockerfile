FROM golang:1.22.2-alpine AS builder

WORKDIR /app

COPY . /app

RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/main.go

RUN go mod tidy

RUN chmod +x main

FROM alpine as certs

RUN apk update && apk add --no-cache ca-certificates

RUN cp -r /etc/ssl/certs /certs

FROM scratch

COPY --from=builder /app/app .

EXPOSE 8080

CMD ["./main"]