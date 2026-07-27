package clientservice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/simpplify-org/GO-simpzap/pkg/alerting"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// WebhookRule representa uma regra de webhook para uma frase específica.
type WebhookRule struct {
	Phrase      string
	CallbackURL string
}

// WhatsAppService encapsula a lógica de conexão e interação com o WhatsApp.
type WhatsAppService struct {
	client      *whatsmeow.Client
	ctx         context.Context
	phoneNumber string
	dbLog       waLog.Logger
	clientLog   waLog.Logger
	dbContainer *sqlstore.Container
	webhooks    map[string][]WebhookRule // Mapeia número de telefone para regras de webhook
	mu          sync.RWMutex             // Mutex para proteger o mapa de webhooks

	stateMu     sync.RWMutex // Mutex para proteger lastEvent/lastEventAt
	lastEvent   string
	lastEventAt time.Time
}

// ConnectionStatus resume o estado da conexão com o WhatsApp para consumo
// pelos serviços externos (via GET /status) e pelo health monitor do master.
// Existe para que quem chama /send não fique "no escuro" quando a conexão
// cai: em vez de só receber um erro genérico, dá pra checar antecipadamente
// (ou interpretar o campo "reason" do erro de /send) se é uma queda
// transitória (NeedsQR=false, o processo reconecta sozinho) ou se exige
// intervenção humana (NeedsQR=true, sessão foi deslogada de vez).
type ConnectionStatus struct {
	PhoneNumber string    `json:"phone_number"`
	Connected   bool      `json:"connected"`
	LoggedIn    bool      `json:"logged_in"`
	HasSession  bool      `json:"has_session"`
	NeedsQR     bool      `json:"needs_qr"`
	LastEvent   string    `json:"last_event,omitempty"`
	LastEventAt time.Time `json:"last_event_at"`
}

// NewWhatsAppService é o construtor para WhatsAppService.
func NewWhatsAppService(ctx context.Context, phoneNumber string) (*WhatsAppService, error) {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	clientLog := waLog.Stdout("Client", "DEBUG", true)

	// "data/" fica num volume Docker dedicado (ver docker_manager.go), para que
	// a sessão sobreviva a crash/recriação do container sem exigir novo QR.
	if err := os.MkdirAll("data", 0755); err != nil {
		return nil, fmt.Errorf("erro ao criar diretório data: %w", err)
	}

	container, err := sqlstore.New(ctx, "sqlite3", "file:data/device.db?_foreign_keys=on", dbLog)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir sqlstore: %w", err)
	}

	service := &WhatsAppService{
		ctx:         ctx,
		phoneNumber: phoneNumber,
		dbLog:       dbLog,
		clientLog:   clientLog,
		dbContainer: container,
		webhooks:    make(map[string][]WebhookRule),
	}

	err = service.initClient()
	if err != nil {
		return nil, err
	}

	service.client.AddEventHandler(service.eventHandler)

	return service, nil
}

func (s *WhatsAppService) initClient() error {
	jid, err := types.ParseJID(s.phoneNumber + types.DefaultUserServer)
	if err != nil {
		return fmt.Errorf("JID inválido: %w", err)
	}

	deviceStore, err := s.dbContainer.GetDevice(s.ctx, jid)
	if err != nil {
		return fmt.Errorf("erro ao obter device: %w", err)
	}
	if deviceStore == nil {
		// Cria um device novo vazio — não reutiliza sessão de outro número
		deviceStore = s.dbContainer.NewDevice()
	}

	s.client = whatsmeow.NewClient(deviceStore, s.clientLog)
	return nil
}

// Connect estabelece a conexão com o WhatsApp.
func (s *WhatsAppService) Connect() error {
	return s.client.Connect()
}

// Disconnect encerra a conexão com o WhatsApp.
func (s *WhatsAppService) Disconnect() {
	s.client.Disconnect()
}

// IsConnected verifica se o cliente está conectado.
func (s *WhatsAppService) IsConnected() bool {
	return s.client.IsConnected()
}

// GetQRChannel retorna o canal QR para autenticação.
func (s *WhatsAppService) GetQRChannel() (<-chan whatsmeow.QRChannelItem, error) {
	return s.client.GetQRChannel(s.ctx)
}

// HasID verifica se o cliente já possui uma ID de sessão.
func (s *WhatsAppService) HasID() bool {
	return s.client.Store.ID != nil
}

// Status retorna o estado atual da conexão. NeedsQR reflete diretamente
// client.Store.ID: o whatsmeow apaga o store automaticamente quando recebe
// um LoggedOut real (device removido / logout no celular), então "sem
// sessão" e "precisa de novo QR" são a mesma coisa — não é preciso rastrear
// isso manualmente.
func (s *WhatsAppService) Status() ConnectionStatus {
	s.stateMu.RLock()
	lastEvent, lastEventAt := s.lastEvent, s.lastEventAt
	s.stateMu.RUnlock()

	hasSession := s.HasID()
	return ConnectionStatus{
		PhoneNumber: s.phoneNumber,
		Connected:   s.IsConnected(),
		LoggedIn:    s.client.IsLoggedIn(),
		HasSession:  hasSession,
		NeedsQR:     !hasSession,
		LastEvent:   lastEvent,
		LastEventAt: lastEventAt,
	}
}

func (s *WhatsAppService) setLastEvent(name string) {
	s.stateMu.Lock()
	s.lastEvent = name
	s.lastEventAt = time.Now()
	s.stateMu.Unlock()
}

// SendMessage envia uma mensagem de texto para um número.
func (s *WhatsAppService) SendMessage(number, message string) (whatsmeow.SendResponse, error) {
	if !s.IsConnected() {
		return whatsmeow.SendResponse{}, fmt.Errorf("cliente WhatsApp não conectado")
	}

	jid := types.NewJID(number, types.DefaultUserServer)
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(message),
		},
	}

	resp, err := s.client.SendMessage(s.ctx, jid, msg)
	if err != nil {
		return whatsmeow.SendResponse{}, fmt.Errorf("erro ao enviar mensagem para %s: %w", number, err)
	}

	return resp, nil
}

// sendInternalMessage é uma função auxiliar para enviar mensagens de status internas.
func (s *WhatsAppService) sendInternalMessage(number, message string) {
	resp, err := s.SendMessage(number, message)
	if err != nil {
		log.Printf("❌ Erro ao enviar mensagem interna para %s: %v\n", number, err)
		return
	}
	log.Printf("✅ Mensagem interna enviada para %s (ID: %s)\n", number, resp.ID)
}

// eventHandler manipula os eventos do WhatsApp.
func (s *WhatsAppService) eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		s.handleMessageEvent(v)
	case *events.Connected:
		s.setLastEvent("connected")
		log.Println("✅ WhatsApp conectado com sucesso!")
	case *events.Disconnected:
		s.setLastEvent("disconnected")
		log.Println("❌ WhatsApp desconectado! (reconexão automática do whatsmeow deve assumir, se a sessão ainda for válida)")
	case *events.StreamReplaced:
		s.setLastEvent("stream_replaced")
		log.Println("⚠️ Sessão substituída em outro dispositivo.")
		alerting.CaptureSessionDown(s.phoneNumber, "stream_replaced", "sessão foi substituída por login em outro lugar — esta conexão foi encerrada")
	case *events.LoggedOut:
		s.setLastEvent("logged_out")
		log.Println("🚪 Logout realizado — sessão expirada, será necessário escanear um novo QR Code.")
		alerting.CaptureSessionDown(s.phoneNumber, "logged_out", "sessão deslogada — é necessário escanear um novo QR Code em /connect/ws")
	default:
		// log.Printf("🌀 Evento: %+v\n", v) // Comentado para reduzir o ruído do log
	}
}

// handleMessageEvent processa eventos de mensagem e dispara webhooks.
func (s *WhatsAppService) handleMessageEvent(v *events.Message) {
	number := v.Info.Sender.User
	text := v.Message.GetConversation()

	fmt.Printf("[%s] %s\n", number, text)

	s.mu.RLock()
	rules, ok := s.webhooks[number]
	s.mu.RUnlock()

	if ok {
		for _, rule := range rules {
			if rule.Phrase == text {
				go s.dispatchWebhook(rule, number, text)
			}
		}
	}
}

// dispatchWebhook envia a requisição para o URL do webhook.
func (s *WhatsAppService) dispatchWebhook(rule WebhookRule, number, text string) {
	body := map[string]string{
		"number":  number,
		"message": text,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		log.Printf("Erro ao serializar payload do webhook: %v", err)
		return
	}

	s.sendInternalMessage(number, "Solicitação recebida, aguarde...")

	resp, err := http.Post(rule.CallbackURL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("Erro ao chamar webhook %s: %v", rule.CallbackURL, err)
		s.sendInternalMessage(number, "Não foi possível se comunicar com o servidor intermediário: "+rule.CallbackURL)
		return
	}
	defer resp.Body.Close()

	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		log.Printf("Aviso: Não foi possível ler o corpo da resposta do webhook %s: %v", rule.CallbackURL, readErr)
	}

	var jsonMap map[string]interface{}
	unmarshalErr := json.Unmarshal(responseBody, &jsonMap)

	responseBodyString := ""

	if unmarshalErr == nil {
		formattedJSON, marshalErr := json.MarshalIndent(jsonMap, "", "  ")

		if marshalErr == nil {
			responseBodyString = string(formattedJSON)
		} else {
			responseBodyString = string(responseBody)
			log.Printf("Aviso: Erro ao re-codificar JSON formatado do webhook %s: %v", rule.CallbackURL, marshalErr)
		}
	} else {
		responseBodyString = string(responseBody)
		log.Printf("Aviso: Corpo da resposta do webhook %s não parece ser JSON válido: %v", rule.CallbackURL, unmarshalErr)
	}

	message := fmt.Sprintf("Solicitação enviada para o servidor!\n Status HTTP: %d", resp.StatusCode)

	if len(responseBodyString) > 0 {
		message += fmt.Sprintf("\nResposta do Servidor:\n%s", responseBodyString)
	}

	s.sendInternalMessage(number, message)
	log.Printf("Webhook %s disparado! Status: %d. Resposta: %s", rule.CallbackURL, resp.StatusCode, responseBodyString)
}

// RegisterWebhook adiciona uma nova regra de webhook.
func (s *WhatsAppService) RegisterWebhook(number, phrase, callbackURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.webhooks[number] = append(s.webhooks[number], WebhookRule{
		Phrase:      phrase,
		CallbackURL: callbackURL,
	})
}

// ListWebhooks retorna o mapa de webhooks.
func (s *WhatsAppService) ListWebhooks() map[string][]WebhookRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Retorna uma cópia para evitar modificações externas
	webhooksCopy := make(map[string][]WebhookRule)
	for k, v := range s.webhooks {
		webhooksCopy[k] = v
	}
	return webhooksCopy
}

// DeleteWebhook remove um webhook específico baseado no número, frase e URL.
func (s *WhatsAppService) DeleteWebhook(number, phrase, callbackURL string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	rules, exists := s.webhooks[number]
	if !exists {
		return false
	}

	updated := make([]WebhookRule, 0)
	removed := false

	for _, r := range rules {
		if r.Phrase == phrase && r.CallbackURL == callbackURL {
			removed = true
			continue // não adiciona (remove)
		}
		updated = append(updated, r)
	}

	if removed {
		s.webhooks[number] = updated

		// Se ficou vazio, remove totalmente a chave
		if len(updated) == 0 {
			delete(s.webhooks, number)
		}
	}

	return removed
}

// EncodeQRToDataURL converte o código QR em uma URL de dados base64.
func EncodeQRToDataURL(qrCode string) (string, error) {
	img, err := qrcode.Encode(qrCode, qrcode.Medium, 180)
	if err != nil {
		return "", fmt.Errorf("erro ao codificar QR Code: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(img)
	return "data:image/png;base64," + encoded, nil
}
