FROM golang:1.24.4-alpine AS builder

RUN apk add --no-cache gcc g++ make sqlite-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

WORKDIR /app/cmd/client
ENV CGO_ENABLED=1
RUN go build -o /zap-client .

# ------------------------
# Final
# ------------------------
FROM alpine:3.19
WORKDIR /app

RUN apk add --no-cache sqlite-libs

COPY --from=builder /zap-client .

ENV PHONE_NUMBER=default

EXPOSE 8080
ENTRYPOINT ["/app/zap-client"]
