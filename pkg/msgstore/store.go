// Package msgstore grava no Postgres o histórico de mensagens trocadas por
// cada device (recebidas, enviadas pelo celular e enviadas via API), para
// termos volume real de mensagens por conversa/mês.
package msgstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Message é uma mensagem do WhatsApp já normalizada para gravação.
type Message struct {
	DevicePhone string    // número do device (PHONE_NUMBER do container)
	MessageID   string    // ID da mensagem no WhatsApp
	ChatJID     string    // conversa (usuário ou grupo)
	SenderJID   string    // quem enviou
	FromMe      bool      // true = enviada pelo device (nós), false = recebida
	IsGroup     bool      // conversa em grupo
	MessageType string    // text, image, video, audio, document, sticker, reaction, ...
	Text        string    // texto/legenda, quando houver
	SentAt      time.Time // timestamp da mensagem no WhatsApp
	Source      string    // live, history ou api
}

type Store struct {
	db *sql.DB
}

func New(dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// migrate é idempotente: todos os containers filhos rodam isso no boot.
func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS whatsapp_messages (
			id           BIGSERIAL PRIMARY KEY,
			device_phone VARCHAR(20)  NOT NULL,
			message_id   VARCHAR(128) NOT NULL,
			chat_jid     VARCHAR(128) NOT NULL,
			sender_jid   VARCHAR(128) NOT NULL,
			from_me      BOOLEAN      NOT NULL,
			is_group     BOOLEAN      NOT NULL DEFAULT FALSE,
			message_type VARCHAR(32)  NOT NULL,
			text         TEXT,
			sent_at      TIMESTAMPTZ  NOT NULL,
			source       VARCHAR(16)  NOT NULL,
			created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
			UNIQUE (device_phone, chat_jid, message_id)
		);
		CREATE INDEX IF NOT EXISTS idx_whatsapp_messages_device_sent_at
			ON whatsapp_messages (device_phone, sent_at);
		CREATE INDEX IF NOT EXISTS idx_whatsapp_messages_chat_sent_at
			ON whatsapp_messages (chat_jid, sent_at);
	`)
	return err
}

// Save grava as mensagens numa única transação. Mensagens repetidas (mesmo
// device/chat/id — ex.: history sync de algo que já veio ao vivo) são
// ignoradas.
func (s *Store) Save(ctx context.Context, msgs ...Message) error {
	if len(msgs) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO whatsapp_messages
			(device_phone, message_id, chat_jid, sender_jid, from_me, is_group, message_type, text, sent_at, source)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10)
		ON CONFLICT (device_phone, chat_jid, message_id) DO NOTHING
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range msgs {
		if _, err := stmt.ExecContext(ctx,
			m.DevicePhone, m.MessageID, m.ChatJID, m.SenderJID, m.FromMe, m.IsGroup,
			m.MessageType, m.Text, m.SentAt, m.Source,
		); err != nil {
			return fmt.Errorf("erro ao gravar mensagem %s: %w", m.MessageID, err)
		}
	}

	return tx.Commit()
}

func (s *Store) Close() error {
	return s.db.Close()
}
