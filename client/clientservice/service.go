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
	"sync"
	"time"

	"github.com/skip2/go-qrcode"

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
	UrlMethod   string
	Body        string
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
}

// NewWhatsAppService é o construtor para WhatsAppService.
func NewWhatsAppService(ctx context.Context, phoneNumber string) (*WhatsAppService, error) {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	clientLog := waLog.Stdout("Client", "DEBUG", true)

	container, err := sqlstore.New(ctx, "sqlite3", "file:device.db?_foreign_keys=on", dbLog)
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
		deviceStore, err = s.dbContainer.GetFirstDevice(s.ctx)
		if err != nil {
			return fmt.Errorf("erro ao criar device: %w", err)
		}
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
		log.Println("✅ WhatsApp conectado com sucesso!")
	case *events.Disconnected:
		log.Println("❌ WhatsApp desconectado!")
	case *events.StreamReplaced:
		log.Println("⚠️ Sessão substituída em outro dispositivo.")
	case *events.LoggedOut:
		log.Println("🚪 Logout realizado — sessão expirada.")
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
				go s.dispatchWebhook(rule, number)
			}
		}
	}
}

// dispatchWebhook envia a requisição para o URL do webhook.
func (s *WhatsAppService) dispatchWebhook(rule WebhookRule, number string) {
	payload := []byte(rule.Body)
	s.sendInternalMessage(number, "Solicitação recebida, aguarde...")
	req, err := http.NewRequest(rule.UrlMethod, rule.CallbackURL, bytes.NewBuffer(payload))
	if err != nil {
		log.Printf("Erro ao criar request para webhook %s: %v", rule.CallbackURL, err)
		s.sendInternalMessage(number, "Erro ao criar requisição: "+rule.CallbackURL)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Erro ao chamar webhook %s: %v", rule.CallbackURL, err)
		s.sendInternalMessage(number, "Não foi possível se comunicar com o servidor: "+rule.CallbackURL)
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
func (s *WhatsAppService) RegisterWebhook(number, phrase, callbackURL, urlMethod, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.webhooks[number] = append(s.webhooks[number], WebhookRule{
		Phrase:      phrase,
		CallbackURL: callbackURL,
		UrlMethod:   urlMethod,
		Body:        body,
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
