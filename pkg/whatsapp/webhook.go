package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// WebhookRegister representa regras de webhook para uma frase específica.
type WebhookRegister struct {
	Phrase      string
	CallbackURL string
	UrlMethod   string
	Number      string
	Body        string
}

type WebhookRule struct {
	Phrase      string `json:"Phrase"`
	CallbackURL string `json:"CallbackURL"`
	UrlMethod   string `json:"UrlMethod,omitempty"`
	Body        string `json:"Body,omitempty"`
}

// hooks: lista de regras que serão registradas
func (s *ZapPkg) PushWebhooks(ctx context.Context, cc *ClientContainer, hooks []WebhookRegister) error {
	if cc == nil {
		return fmt.Errorf("client container é nil")
	}
	base := cc.Endpoint
	if base == "" {
		return fmt.Errorf("endpoint do container vazio")
	}

	existing, err := fetchExistingWebhooks(ctx, base)
	if err != nil {
		return fmt.Errorf("erro ao buscar webhooks existentes: %w", err)
	}

	// chave: phrase|callback|method|number
	existingSet := make(map[string]struct{}, len(existing))
	for _, e := range existing {
		key := webhookKey(e.Phrase, e.CallbackURL, e.UrlMethod, e.Number)
		existingSet[key] = struct{}{}
	}

	for _, hr := range hooks {
		key := webhookKey(hr.Phrase, hr.CallbackURL, hr.UrlMethod, hr.Number)
		if _, ok := existingSet[key]; ok {
			log.Printf("[PushWebhooks] já existe (phrase=%s callback=%s method=%s) — pulando", hr.Phrase, hr.CallbackURL, hr.UrlMethod)
			continue
		}

		if err := registerWebhook(ctx, base, hr); err != nil {
			return fmt.Errorf("erro ao registrar webhook (phrase=%s callback=%s): %w", hr.Phrase, hr.CallbackURL, err)
		}
		log.Printf("[PushWebhooks] webhook registrado (phrase=%s callback=%s method=%s)", hr.Phrase, hr.CallbackURL, hr.UrlMethod)

		existingSet[key] = struct{}{}
	}

	return nil
}

// fetchExistingWebhooks chama POST /webhook/list e decodifica a lista
func fetchExistingWebhooks(ctx context.Context, baseURL string) ([]WebhookRegister, error) {
	url := fmt.Sprintf("%s/webhook/list", trimTrailingSlash(baseURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro http ao chamar %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d ao listar webhooks: %s", resp.StatusCode, string(b))
	}

	var result map[string][]WebhookRule
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("erro decode body map webhooks: %w", err)
	}
	return mapToWebhookRegisters(result), nil
}

// registerWebhook chama POST /webhook/register com o payload apropriado.
func registerWebhook(ctx context.Context, baseURL string, hr WebhookRegister) error {
	url := fmt.Sprintf("%s/webhook/register", trimTrailingSlash(baseURL))

	payload := map[string]string{
		"number":       hr.Number,
		"phrase":       hr.Phrase,
		"callback_url": hr.CallbackURL,
		"url_method":   hr.UrlMethod,
		"body":         hr.Body,
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("erro http post register: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registro retornou status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func mapToWebhookRegisters(m map[string][]WebhookRule) []WebhookRegister {
	result := make([]WebhookRegister, 0)

	for number, rules := range m {
		for _, rule := range rules {
			result = append(result, WebhookRegister{
				Phrase:      rule.Phrase,
				CallbackURL: rule.CallbackURL,
				UrlMethod:   rule.UrlMethod,
				Number:      number,
				Body:        rule.Body,
			})
		}
	}

	return result
}

// webhookKey gera chave para comparação exata (phrase|callback|method|number)
func webhookKey(phrase, callback, method, number string) string {
	return fmt.Sprintf("%s|%s|%s|%s", phrase, callback, method, number)
}

// trimTrailingSlash remove slash final para montar URLs corretamente
func trimTrailingSlash(s string) string {
	if s == "" {
		return s
	}
	if s[len(s)-1] == '/' {
		return s[:len(s)-1]
	}
	return s
}
