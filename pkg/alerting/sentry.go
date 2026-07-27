// Package alerting centraliza o envio de alertas para o Sentry quando uma
// sessão do WhatsApp cai. O objetivo é que, além dos reconnects automáticos
// (whatsmeow + auto-connect no boot + health monitor do master), o time seja
// notificado instantaneamente quando algo exigir intervenção humana — em vez
// de descobrir a queda só quando um cliente reclamar que parou de receber
// mensagem.
package alerting

import (
	"fmt"
	"log"
	"time"

	"github.com/getsentry/sentry-go"
)

// Init inicializa o SDK do Sentry a partir do DSN informado. Se dsn estiver
// vazio, a integração fica desligada (CaptureSessionDown vira no-op) — assim
// o alerta é opcional e não quebra ambientes (dev/homolog) sem Sentry
// configurado.
func Init(dsn, environment, serverName string) bool {
	if dsn == "" {
		log.Println("[Sentry] SENTRY_DSN não definida — alertas de queda de sessão desativados.")
		return false
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: environment,
		ServerName:  serverName,
	})
	if err != nil {
		log.Printf("[Sentry] falha ao inicializar (alertas ficarão desativados): %v", err)
		return false
	}

	log.Println("[Sentry] inicializado — quedas de sessão do WhatsApp serão reportadas como issue grave.")
	return true
}

// Flush aguarda o envio dos eventos pendentes ao Sentry. Deve ser chamado
// antes do processo encerrar (defer no main), senão eventos gerados perto do
// shutdown podem ser perdidos.
func Flush(timeout time.Duration) {
	sentry.Flush(timeout)
}

// CaptureSessionDown reporta ao Sentry, como issue de nível Fatal, a queda da
// sessão do WhatsApp de um device. O fingerprint agrupa por
// telefone+motivo para que quedas repetidas do mesmo device virem uma única
// issue (com contador de ocorrências) em vez de spam de issues novas.
func CaptureSessionDown(phoneNumber, reason, message string) {
	if sentry.CurrentHub().Client() == nil {
		return
	}

	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelFatal)
		scope.SetTag("phone_number", phoneNumber)
		scope.SetTag("reason", reason)
		scope.SetFingerprint([]string{"whatsapp-session-down", phoneNumber, reason})
		sentry.CaptureMessage(fmt.Sprintf("[WhatsApp] Sessão caiu — device %s (%s): %s", phoneNumber, reason, message))
	})
}
