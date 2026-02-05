# 📲 WhatsApp Multi-Device API (Go + Whatsmeow)

Este projeto é uma solução **escalável** para integração com WhatsApp.  
Ele utiliza uma arquitetura de **Orquestração de Instâncias**, onde um serviço **Master** gerencia containers Docker independentes (**filhos**) para cada novo dispositivo, garantindo **isolamento total**, **persistência de sessão** e **estabilidade**.

---

## 🏗️ Arquitetura

O sistema opera através de um fluxo de criação dinâmica:

1. **Master Service**
    - Recebe solicitações de criação de novos devices
    - Cria e gerencia containers Docker dedicados

2. **Instance Service**
    - Cada container filho representa **um número do WhatsApp**
    - Gerencia:
        - Conexão WebSocket
        - QR Code
        - Envio de mensagens
        - Persistência de sessão

```
[ Client ] 
    |
    v
[ Master Service ]
    |
    +--> [ Instance Container #1 ] (Device A)
    +--> [ Instance Container #2 ] (Device B)
    +--> [ Instance Container #N ] (Device N)
```

---

## 🚀 Como Rodar (Master)

### 📋 Pré-requisitos

- Go **1.20+**
- Docker
- Docker Compose
- SQLite3

---

### 🔧 Instalação

1. Clone o repositório:

```bash
git clone https://github.com/seu-usuario/seu-repo.git
cd seu-repo
```

2. Instale as dependências:

```bash
go mod tidy
```

3. Inicie o serviço principal (Master):

```bash
go run cmd/main.go
```

---

## 🛠️ Fluxo de Utilização

### 1️⃣ Criar uma Nova Instância (Device)

Para gerar um container filho para um número específico, utilize o endpoint do **Master**:

```http
POST http://localhost:8080/create
```

Ou via servidor remoto:

```http
POST http://52.23.179.22:3372/create
```

#### 📥 Request Body

```json
{
  "number": "11999999999"
}
```

#### 📤 Response

```json
{
  "status": "created",
  "endpoint": "http://52.23.179.22:36945",
  "id": "cbc5fa8b4efb5dadda097665fa885e81793265b09083858010af8eead25b98e9"
}
```

📌 **Nota:**  
O `endpoint` retornado é **exclusivo** dessa instância e deve ser utilizado para todas as interações com este device.

---

### 2️⃣ Autenticação via WebSocket (QR Code)

Após criar a instância, conecte-se via WebSocket para obter o QR Code:

```text
ws://52.23.179.22:36945/connect/ws
```

- O QR Code será enviado pelo WebSocket
- Após escanear com o WhatsApp, a sessão ficará persistida
- O container manterá a conexão ativa automaticamente

---

### 3️⃣ Envio de Mensagens (Na Instância)

Todos os envios devem ser feitos **diretamente no endpoint da instância criada**.

---

#### 📩 Envio Unitário

```http
POST /send
```

```json
{
  "number": "5511999999999",
  "message": "Olá! Mensagem de teste via container dedicado 🚀"
}
```

---

#### 📬 Envio em Massa

```http
POST /send/many
```

```json
{
  "numbers": [
    "5511999999999",
    "5511888888888"
  ],
  "message": "Mensagem importante para múltiplos contatos."
}
```

---

## 📦 Gerenciamento Docker

Cada device roda em um **container isolado**, o que permite:

- 🔒 Isolamento total entre contas
- 💥 Falha de um número não afeta os demais
- 📈 Escalabilidade horizontal
- 🧠 Limitação de memória por container
- 🔄 Atualizações granulares de código

---

## 💾 Persistência de Sessão

- As sessões são armazenadas em um arquivo `session.db`
- Cada container possui seu próprio volume Docker
- As sessões permanecem ativas mesmo após reinício do container

---

## ⚠️ Observações Importantes

- 📱 Recomenda-se que o número de destino já tenha tido interações prévias
- 🚫 Evite envios em massa agressivos para reduzir risco de bloqueios
- 🔁 Um container representa **exatamente um número**

---

## 📄 Licença

Este projeto está sob a licença **MIT**.
