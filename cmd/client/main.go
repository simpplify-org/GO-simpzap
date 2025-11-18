package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/simpplify-org/GO-simpzap/cmd/client/clientservice"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/websocket"
)

var (
	ctx         = context.Background()
	phoneNumber = os.Getenv("PHONE_NUMBER")
	service     *clientservice.WhatsAppService
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleConnectWS → mantém o socket aberto para exibir QR e status
func handleConnectWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("❌ Erro ao iniciar WebSocket:", err)
		return
	}
	defer conn.Close()

	if service == nil {
		conn.WriteJSON(map[string]string{"error": "Serviço WhatsApp não inicializado"})
		return
	}

	if !service.HasID() {
		qrChan, err := service.GetQRChannel()
		if err != nil {
			conn.WriteJSON(map[string]string{"error": err.Error()})
			return
		}

		go func() {
			// mantém o client conectado
			if err := service.Connect(); err != nil {
				conn.WriteJSON(map[string]string{"error": err.Error()})
				return
			}
		}()

		for evt := range qrChan {
			switch evt.Event {
			case "code":
				dataURL, err := clientservice.EncodeQRToDataURL(evt.Code)
				if err != nil {
					log.Printf("Erro ao codificar QR: %v", err)
					conn.WriteJSON(map[string]string{"error": "Erro ao gerar QR Code"})
					return
				}
				conn.WriteJSON(map[string]any{
					"event": "qr",
					"image": dataURL,
				})
			case "success":
				conn.WriteJSON(map[string]any{"event": "success"})
			case "timeout":
				conn.WriteJSON(map[string]any{"event": "timeout"})
			}
		}
	} else {
		if err := service.Connect(); err != nil {
			conn.WriteJSON(map[string]string{"error": err.Error()})
			return
		}
		conn.WriteJSON(map[string]string{"event": "reconnected"})
	}
}

// handleSendMessage - POST /send — envia para um número
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

	if service == nil || !service.IsConnected() {
		http.Error(w, "Cliente WhatsApp não conectado", http.StatusServiceUnavailable)
		return
	}

	log.Printf("📤 Enviando mensagem para %s...\n", req.Number)

	resp, err := service.SendMessage(req.Number, req.Message)
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

// handleSendManyMessages - POST /send/many — envia mesma mensagem para vários números
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

	if service == nil || !service.IsConnected() {
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

		resp, err := service.SendMessage(number, req.Message)
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

// handleRegisterWebhook - POST /webhook/register — registra um webhook de um numero
func handleRegisterWebhook(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		Number      string `json:"number"`
		Phrase      string `json:"phrase"`
		CallbackURL string `json:"callback_url"`
	}

	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "payload inválido", http.StatusBadRequest)
		return
	}

	if service == nil {
		http.Error(w, "Serviço WhatsApp não inicializado", http.StatusServiceUnavailable)
		return
	}

	service.RegisterWebhook(req.Number, req.Phrase, req.CallbackURL)

	json.NewEncoder(w).Encode(map[string]string{
		"status": "registered",
	})
}

// handleListWebhooks - POST /webhook/list — lista os webhook
func handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	if service == nil {
		http.Error(w, "Serviço WhatsApp não inicializado", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(service.ListWebhooks())
}

// handleDeleteWebhook - POST /webhook/delete — deleta um webhook de um numero
func handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	type Req struct {
		Number      string `json:"number"`
		Phrase      string `json:"phrase"`
		CallbackURL string `json:"callback_url"`
	}

	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "payload inválido", http.StatusBadRequest)
		return
	}

	if service == nil {
		http.Error(w, "Serviço WhatsApp não inicializado", http.StatusServiceUnavailable)
		return
	}

	service.DeleteWebhook(req.Number, req.Phrase, req.CallbackURL)

	json.NewEncoder(w).Encode(map[string]string{
		"status": "deleted",
	})
}

// main com logs e shutdown gracioso
func main() {
	var err error
	service, err = clientservice.NewWhatsAppService(ctx, phoneNumber)
	if err != nil {
		log.Fatalf("Erro ao inicializar o serviço client WhatsApp: %v", err)
	}

	http.HandleFunc("/connect/ws", handleConnectWS)
	http.HandleFunc("/send", handleSendMessage)
	http.HandleFunc("/send/many", handleSendManyMessages)
	http.HandleFunc("/webhook/register", handleRegisterWebhook)
	http.HandleFunc("/webhook/list", handleListWebhooks)
	http.HandleFunc("/webhook/delete", handleDeleteWebhook)
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
	if service != nil {
		service.Disconnect()
	}
}
