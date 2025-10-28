package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/skip2/go-qrcode"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

var (
	client      *whatsmeow.Client
	ctx         = context.Background()
	phoneNumber = os.Getenv("PHONE_NUMBER")
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Handler de eventos com logs
func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		fmt.Printf("📩 [%s] %s\n", v.Info.Sender.User, v.Message.GetConversation())
	case *events.Connected:
		log.Println("✅ WhatsApp conectado com sucesso!")
	case *events.Disconnected:
		log.Println("❌ WhatsApp desconectado!")
	case *events.StreamReplaced:
		log.Println("⚠️ Sessão substituída em outro dispositivo.")
	case *events.LoggedOut:
		log.Println("🚪 Logout realizado — sessão expirada.")
	default:
		// Log de debug para eventos desconhecidos
		log.Printf("🌀 Evento: %+v\n", v)
	}
}

func initClient() (*whatsmeow.Client, error) {
	if client != nil {
		return client, nil
	}

	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container, err := sqlstore.New(ctx, "sqlite3", "file:device.db?_foreign_keys=on", dbLog)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir sqlstore: %w", err)
	}

	jid, err := types.ParseJID(phoneNumber + types.DefaultUserServer)
	if err != nil {
		return nil, fmt.Errorf("JID inválido: %w", err)
	}

	deviceStore, err := container.GetDevice(ctx, jid)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter device: %w", err)
	}
	if deviceStore == nil {
		deviceStore, err = container.GetFirstDevice(ctx)
		if err != nil {
			return nil, fmt.Errorf("erro ao criar device: %w", err)
		}
	}

	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client = whatsmeow.NewClient(deviceStore, clientLog)
	client.AddEventHandler(eventHandler)

	return client, nil
}

// connect/ws → mantém o socket aberto para exibir QR e status
func handleConnectWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("❌ Erro ao iniciar WebSocket:", err)
		return
	}
	defer conn.Close()

	c, err := initClient()
	if err != nil {
		conn.WriteJSON(map[string]string{"error": err.Error()})
		return
	}

	if c.Store.ID == nil {
		qrChan, _ := c.GetQRChannel(ctx)

		go func() {
			// mantém o client conectado
			if err := c.Connect(); err != nil {
				conn.WriteJSON(map[string]string{"error": err.Error()})
				return
			}
		}()

		for evt := range qrChan {
			switch evt.Event {
			case "code":
				img, _ := qrcode.Encode(evt.Code, qrcode.Medium, 180)
				encoded := base64.StdEncoding.EncodeToString(img)
				conn.WriteJSON(map[string]any{
					"event": "qr",
					"image": "data:image/png;base64," + encoded,
				})
			case "success":
				conn.WriteJSON(map[string]any{"event": "success"})
			case "timeout":
				conn.WriteJSON(map[string]any{"event": "timeout"})
			}
		}
	} else {
		if err := c.Connect(); err != nil {
			conn.WriteJSON(map[string]string{"error": err.Error()})
			return
		}
		conn.WriteJSON(map[string]string{"event": "reconnected"})
	}
}

// POST /send — envia para um número
func handleSendMessage(w http.ResponseWriter, r *http.Request) {
	type SendRequest struct {
		Number  string `json:"number"`
		Message string `json:"message"`
	}

	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	if client == nil || !client.IsConnected() {
		http.Error(w, "Cliente WhatsApp não conectado", http.StatusServiceUnavailable)
		return
	}

	log.Printf("📤 Enviando mensagem para %s...\n", req.Number)
	jid := types.NewJID(req.Number, types.DefaultUserServer)
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(req.Message),
		},
	}

	resp, err := client.SendMessage(ctx, jid, msg)
	if err != nil {
		log.Printf("❌ Erro ao enviar para %s: %v\n", req.Number, err)
		http.Error(w, fmt.Sprintf("Erro ao enviar: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("✅ Mensagem enviada para %s (ID: %s)\n", req.Number, resp.ID)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"id":     resp.ID,
	})
}

// POST /send/many — envia mesma mensagem para vários números
func handleSendManyMessages(w http.ResponseWriter, r *http.Request) {
	type SendManyRequest struct {
		Numbers []string `json:"numbers"`
		Message string   `json:"message"`
	}
	type SendResult struct {
		Number string `json:"number"`
		ID     string `json:"id,omitempty"`
		Error  string `json:"error,omitempty"`
	}

	var req SendManyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	if client == nil || !client.IsConnected() {
		http.Error(w, "Cliente WhatsApp não conectado", http.StatusServiceUnavailable)
		return
	}

	if len(req.Numbers) == 0 {
		http.Error(w, "Nenhum número informado", http.StatusBadRequest)
		return
	}

	results := make([]SendResult, 0, len(req.Numbers))
	for _, number := range req.Numbers {
		log.Printf("📤 Enviando mensagem para %s...\n", number)
		jid := types.NewJID(number, types.DefaultUserServer)
		msg := &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String(req.Message),
			},
		}

		resp, err := client.SendMessage(ctx, jid, msg)
		if err != nil {
			log.Printf("❌ Erro ao enviar para %s: %v\n", number, err)
			results = append(results, SendResult{Number: number, Error: err.Error()})
			continue
		}

		log.Printf("✅ Enviado para %s (ID: %s)\n", number, resp.ID)
		results = append(results, SendResult{Number: number, ID: resp.ID})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"results": results,
	})
}

// main com logs e shutdown gracioso
func main() {
	http.HandleFunc("/connect/ws", handleConnectWS)
	http.HandleFunc("/send", handleSendMessage)
	http.HandleFunc("/send/many", handleSendManyMessages)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	server := &http.Server{Addr: ":8080"}

	go func() {
		log.Println("🚀 Servidor HTTP iniciado em http://localhost:8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Erro no servidor: %v", err)
		}
	}()

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("🧹 Encerrando cliente WhatsApp...")
	if client != nil {
		client.Disconnect()
	}
}
