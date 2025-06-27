# 📲 WhatsApp WebSocket API com Go + Whatsmeow

Este projeto é uma API em Go que integra o WhatsApp via biblioteca [Whatsmeow](https://github.com/tulir/whatsmeow), utilizando WebSocket para comunicação com o cliente. Ele permite:

- 📤 Enviar mensagens via WhatsApp
- 🔄 Manter sessão ativa com reconexão automática
- 🔐 Gerenciar autenticação por QR Code via WebSocket
- 🧠 Persistência de sessão com SQLite

---

## 🚀 Tecnologias

- [Go](https://golang.org/)
- [Whatsmeow](https://github.com/tulir/whatsmeow)
- [Gorilla WebSocket](https://github.com/gorilla/websocket)
- [SQLite](https://www.sqlite.org/index.html)
- [qrterminal](https://github.com/mdp/qrterminal) (para visualização do QR Code no terminal)

---

## 🛠️ Como rodar

### Pré-requisitos

- Go 1.20+
- SQLite3
- Docker (opcional)

### 1. Clonar o repositório:

```bash
git clone https://github.com/seu-usuario/seu-repo.git
cd seu-repo
```

### 2. Instale as dependências:

```bash
go mod tidy
```

## 3. Configure as variáveis de ambiente:

```bash
Crie um arquivo .env com as variáveis de ambiente seguindo o arquivo .env.exemplo
```

## 4. Inicie a aplicação:

```bash
go run cmd/main.go
ou 
make run
```

Acesse `ws://localhost:8080/ws/whatsapp` via WebSocket.

---

## 📦 Docker

### Build da imagem

```bash
docker build -t whatsmeow-app .
```

### Executar

```bash
docker run -p 8080:8080 whatsmeow-app
```

---

## 🔌 WebSocket - Comunicação

### Endpoint

```
ws://localhost:8080/ws/whatsapp
```

### Eventos recebidos do servidor:

| Evento         | Descrição                                     |
|----------------|-----------------------------------------------|
| `code`         | QR Code para escanear                         |
| `connected`    | Sessão conectada com sucesso                  |
| `disconnected` | Conexão perdida                               |
| `restored`     | Sessão restaurada com sucesso                 |
| `message_sent` | Confirmação de envio de mensagem              |
| `send_message` | Evento para enviar mensagens                  |
| `send_error`   | Erro ao enviar mensagem                       |
| `error`        | Erros gerais do servidor                      |

### Envio de mensagem (cliente → servidor)

Formato JSON:

```json
{
  "event": "send_message",
  "to": "5511999999999",
  "text": "Olá! Mensagem de teste 🚀"
}
```


---

## ❗ Observações

- A conta WhatsApp deve estar ativa e o número de destino **precisa ter trocado mensagens anteriormente**.
- A sessão é armazenada no `session.db` para reconexão automática sem novo QR Code.
- Caso a sessão expire, o cliente será notificado via WebSocket com o evento `disconnected`.
- Para rodar localmente, é necessário da biblioteca [godotenv](https://github.com/joho/godotenv), para fazer o load do .env.

---

## 📄 Licença

MIT