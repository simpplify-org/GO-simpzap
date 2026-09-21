package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/simpplify-org/GO-simpzap/cmd/client/clientservice"

	"github.com/gorilla/websocket"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
)

var (
	ctx         = context.Background()
	phoneNumber = os.Getenv("PHONE_NUMBER")
	service     *clientservice.WhatsAppService
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// safeConn serializa escritas no websocket: o gorilla/websocket só permite
// um escritor concorrente por conexão, e este handler escreve tanto da
// goroutine principal (loop do QR) quanto da goroutine de Connect().
type safeConn struct {
	*websocket.Conn
	mu sync.Mutex
}

func (c *safeConn) WriteJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteJSON(v)
}

// closeGracefully envia um close frame de verdade em vez de deixar o TCP
// cair sem handshake (o que o browser reporta como código 1006).
func (c *safeConn) closeGracefully(code int, reason string) {
	c.mu.Lock()
	_ = c.Conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason), time.Now().Add(2*time.Second))
	c.mu.Unlock()
}

// handleConnectWS → mantém o socket aberto para exibir QR e status
func handleConnectWS(w http.ResponseWriter, r *http.Request) {
	rawConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("❌ Erro ao iniciar WebSocket:", err)
		return
	}
	conn := &safeConn{Conn: rawConn}
	defer conn.Close()

	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("❌ Panic no handler de WebSocket, fechando conexão de forma graciosa: %v\n%s", rec, debug.Stack())
			conn.closeGracefully(websocket.CloseInternalServerErr, "erro interno")
		}
	}()

	if service == nil {
		conn.WriteJSON(map[string]string{"error": "Serviço WhatsApp não inicializado"})
		return
	}

	if !service.HasID() {
		// Garante que o client está desconectado antes de abrir o canal QR
		// (evita o erro "GetQRChannel must be called before connecting")
		if service.IsConnected() {
			service.Disconnect()
		}

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

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Libera a origem (para produção, você pode trocar "*" por "http://localhost:3000")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization")

		// Se for uma requisição OPTIONS (Preflight do navegador), retorna 200 OK e para por aqui
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Segue o fluxo normal para a rota solicitada
		next.ServeHTTP(w, r)
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
	http.HandleFunc("/send/media", handleSendMedia)

	server := &http.Server{
		Addr:    ":8080",
		Handler: corsMiddleware(http.DefaultServeMux),
	}

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

const maxMediaSize = 25 << 20

func handleSendMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Metodo nao permitido", http.StatusMethodNotAllowed)
		return
	}

	if service == nil || !service.IsConnected() {
		http.Error(w, "Cliente whatsapp nao conectado", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxMediaSize)

	if err := r.ParseMultipartForm(maxMediaSize); err != nil {
		http.Error(w, "Arquivo invalido ou maior que 25 MB", http.StatusBadRequest)
		return
	}

	number := strings.TrimSpace(r.FormValue("number"))
	caption := strings.TrimSpace(r.FormValue("message"))

	if number == "" {
		http.Error(w, "Numero invalido ou vazio", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormValue("file")
	if err != nil {
		http.Error(w, "Campo file e obrigatorio", http.StatusBadRequest)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Erro ao ler o arquivo", http.StatusInternalServerError)
		return
	}

	if len(data) == 0 {
		http.Error(w "Arquivo vazio", http.StatusBadRequest)
		return
	}

	mimeType := detectMediaType(header, data)

	resp, err := service.SendMedia(
		r.Context(),
		number,
		header.Filename,
		mimeType,
		caption,
		data
	)
	if err != nil {
		log.Printf(
			"❌ Erro ao enviar mídia para %s: %v",
			number,
			err,
		)

		http.Error(
			w,
			fmt.Sprintf("Erro ao enviar mídia: %v", err),
			http.StatusInternalServerError,
		)
		return
	}

	log.Printf(
		"✅ Mídia enviada para %s (ID: %s)",
		number,
		resp.ID,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"id":     string(resp.ID),
	})
}


func detectMediaType(
	header *multipart.FileHeader,
	data []byte,
) string {
	// DetectContentType utiliza os primeiros 512 bytes.
	mimeType := http.DetectContentType(data)

	// Alguns documentos podem ser identificados como octet-stream.
	if mimeType != "application/octet-stream" {
		return mimeType
	}

	switch strings.ToLower(filepath.Ext(header.Filename)) {
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".txt":
		return "text/plain"
	case ".csv":
		return "text/csv"
	default:
		return mimeType
	}
}