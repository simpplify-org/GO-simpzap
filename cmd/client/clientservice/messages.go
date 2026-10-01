package clientservice

import (
	"context"
	"log"
	"time"

	"github.com/simpplify-org/GO-simpzap/pkg/msgstore"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const saveTimeout = 30 * time.Second

// messageType classifica a mensagem pelo conteúdo. Retorna "" para o que não
// é conteúdo trocado com o usuário (protocolo, troca de chaves, etc.), que
// não deve ser gravado.
func messageType(m *waE2E.Message) string {
	switch {
	case m == nil:
		return ""
	case m.GetConversation() != "" || m.GetExtendedTextMessage() != nil:
		return "text"
	case m.GetImageMessage() != nil:
		return "image"
	case m.GetVideoMessage() != nil || m.GetPtvMessage() != nil:
		return "video"
	case m.GetAudioMessage() != nil:
		return "audio"
	case m.GetDocumentMessage() != nil || m.GetDocumentWithCaptionMessage() != nil:
		return "document"
	case m.GetStickerMessage() != nil:
		return "sticker"
	case m.GetReactionMessage() != nil:
		return "reaction"
	case m.GetContactMessage() != nil || m.GetContactsArrayMessage() != nil:
		return "contact"
	case m.GetLocationMessage() != nil || m.GetLiveLocationMessage() != nil:
		return "location"
	case m.GetPollCreationMessage() != nil || m.GetPollCreationMessageV3() != nil || m.GetPollUpdateMessage() != nil:
		return "poll"
	case m.GetButtonsResponseMessage() != nil || m.GetListResponseMessage() != nil ||
		m.GetTemplateButtonReplyMessage() != nil || m.GetInteractiveResponseMessage() != nil:
		return "interactive_reply"
	case m.GetProtocolMessage() != nil || m.GetSenderKeyDistributionMessage() != nil:
		return ""
	default:
		return "other"
	}
}

// messageText extrai o texto ou a legenda da mensagem, quando houver.
func messageText(m *waE2E.Message) string {
	switch {
	case m.GetConversation() != "":
		return m.GetConversation()
	case m.GetExtendedTextMessage() != nil:
		return m.GetExtendedTextMessage().GetText()
	case m.GetImageMessage() != nil:
		return m.GetImageMessage().GetCaption()
	case m.GetVideoMessage() != nil:
		return m.GetVideoMessage().GetCaption()
	case m.GetDocumentMessage() != nil:
		return m.GetDocumentMessage().GetCaption()
	case m.GetReactionMessage() != nil:
		return m.GetReactionMessage().GetText()
	}
	return ""
}

// toStoredMessage converte um evento do whatsmeow para gravação. ok=false
// para mensagens que não interessam (status/broadcast, protocolo).
func (s *WhatsAppService) toStoredMessage(v *events.Message, source string) (msgstore.Message, bool) {
	if v.Info.Chat.Server == types.BroadcastServer {
		return msgstore.Message{}, false
	}
	msgType := messageType(v.Message)
	if msgType == "" {
		return msgstore.Message{}, false
	}

	return msgstore.Message{
		DevicePhone: s.phoneNumber,
		MessageID:   v.Info.ID,
		ChatJID:     v.Info.Chat.ToNonAD().String(),
		SenderJID:   v.Info.Sender.ToNonAD().String(),
		FromMe:      v.Info.IsFromMe,
		IsGroup:     v.Info.IsGroup,
		MessageType: msgType,
		Text:        messageText(v.Message),
		SentAt:      v.Info.Timestamp,
		Source:      source,
	}, true
}

// saveSentMessage grava mensagens enviadas pela API (/send, /send/many,
// /send/media): o whatsmeow não gera events.Message para o que o próprio
// client envia.
func (s *WhatsAppService) saveSentMessage(to types.JID, msg *waE2E.Message, resp whatsmeow.SendResponse) {
	msgType := messageType(msg)
	if msgType == "" {
		return
	}
	s.saveMessages(msgstore.Message{
		DevicePhone: s.phoneNumber,
		MessageID:   resp.ID,
		ChatJID:     to.ToNonAD().String(),
		SenderJID:   s.client.Store.GetJID().ToNonAD().String(),
		FromMe:      true,
		IsGroup:     to.Server == types.GroupServer,
		MessageType: msgType,
		Text:        messageText(msg),
		SentAt:      resp.Timestamp,
		Source:      "api",
	})
}

// saveMessages grava sem nunca derrubar o fluxo de mensagens: falha no banco
// só gera log.
func (s *WhatsAppService) saveMessages(msgs ...msgstore.Message) {
	if s.msgStore == nil || len(msgs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, saveTimeout)
	defer cancel()
	if err := s.msgStore.Save(ctx, msgs...); err != nil {
		log.Printf("❌ Erro ao gravar %d mensagem(ns) no Postgres: %v\n", len(msgs), err)
	}
}

// handleHistorySync grava o histórico que o WhatsApp envia logo após o
// pareamento (QR). É a única forma de recuperar mensagens anteriores à
// existência desta gravação.
func (s *WhatsAppService) handleHistorySync(v *events.HistorySync) {
	if s.msgStore == nil {
		return
	}

	var msgs []msgstore.Message
	for _, conv := range v.Data.GetConversations() {
		chatJID, err := types.ParseJID(conv.GetID())
		if err != nil {
			continue
		}
		for _, hm := range conv.GetMessages() {
			evt, err := s.client.ParseWebMessage(chatJID, hm.GetMessage())
			if err != nil {
				continue
			}
			if m, ok := s.toStoredMessage(evt, "history"); ok {
				msgs = append(msgs, m)
			}
		}
	}

	s.saveMessages(msgs...)
	log.Printf("📚 History sync (%s): %d mensagem(ns) gravada(s)\n", v.Data.GetSyncType(), len(msgs))
}
