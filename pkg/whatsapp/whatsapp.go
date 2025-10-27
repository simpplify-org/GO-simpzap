package whatsapp

//
//import (
//	"context"
//	"fmt"
//	_ "github.com/mattn/go-sqlite3"
//	"github.com/mdp/qrterminal/v3"
//	"go.mau.fi/whatsmeow"
//	"go.mau.fi/whatsmeow/appstate"
//	"go.mau.fi/whatsmeow/proto/waE2E"
//	"go.mau.fi/whatsmeow/store/sqlstore"
//	"go.mau.fi/whatsmeow/types"
//	waLog "go.mau.fi/whatsmeow/util/log"
//	"google.golang.org/protobuf/proto"
//	"log"
//	"os"
//)
//
//var (
//	dbLog     = waLog.Stdout("Database", "DEBUG", true)
//	clientLog = waLog.Stdout("Client", "DEBUG", true)
//)
//
//func CreateClient(deviceID string) (*whatsmeow.Client, <-chan whatsmeow.QRChannelItem, string, error) {
//	ctx := context.Background()
//	sessionPath := fmt.Sprintf(".data/session-%s.db", deviceID)
//
//	container, err := sqlstore.New(ctx, "sqlite3", "file:"+sessionPath+"?_foreign_keys=on", dbLog)
//	if err != nil {
//		return nil, nil, "", fmt.Errorf("erro ao criar sqlstore: %w", err)
//	}
//
//	deviceStore, err := container.GetFirstDevice(ctx)
//	if err != nil {
//		return nil, nil, "", fmt.Errorf("erro ao obter device: %w", err)
//	}
//
//	client := whatsmeow.NewClient(deviceStore, clientLog)
//
//	var qrChan <-chan whatsmeow.QRChannelItem
//	if client.Store.ID == nil {
//		qrChan, _ = client.GetQRChannel(ctx)
//	}
//
//	if err := client.Connect(); err != nil {
//		return nil, nil, "", fmt.Errorf("erro ao conectar no WhatsApp: %w", err)
//	}
//	return client, qrChan, sessionPath, nil
//}
//
//// 2. Inicia client a partir do conteúdo binário da session (arquivo .db)
//// grava arquivo temporário, cria client, conecta e apaga arquivo depois
//func StartClient(sessionData []byte, deviceID string) (client *whatsmeow.Client, qrcode <-chan whatsmeow.QRChannelItem, error error, path string, ctx context.Context) {
//	ctx = context.Background()
//	tmpFile := fmt.Sprintf(".data/session-temp-%s.db", deviceID)
//	if err := os.WriteFile(tmpFile, sessionData, 0644); err != nil {
//		return nil, nil, fmt.Errorf("erro ao gravar arquivo temporário: %w", err), tmpFile, ctx
//	}
//
//	container, err := sqlstore.New(ctx, "sqlite3", "file:"+tmpFile+"?_foreign_keys=on", dbLog)
//	if err != nil {
//		errRemove := os.Remove(tmpFile)
//		if errRemove != nil {
//			return nil, nil, fmt.Errorf("erro ao remover arquivo temp: %w", errRemove), tmpFile, ctx
//		}
//		return nil, nil, fmt.Errorf("erro ao criar sqlstore: %w", err), tmpFile, ctx
//	}
//
//	deviceStore, err := container.GetFirstDevice(ctx)
//	if err != nil {
//		errRemove := os.Remove(tmpFile)
//		if errRemove != nil {
//			return nil, nil, fmt.Errorf("erro ao remover arquivo temp: %w", errRemove), tmpFile, ctx
//		}
//		return nil, nil, fmt.Errorf("erro ao obter device: %w", err), tmpFile, ctx
//	}
//
//	client = whatsmeow.NewClient(deviceStore, clientLog)
//	var qr <-chan whatsmeow.QRChannelItem
//	if client.Store.ID == nil {
//		qr, _ = client.GetQRChannel(context.Background())
//
//		go func() {
//			for evt := range qr {
//				if evt.Event == "code" {
//					fmt.Println("Scan this QR code to log in:")
//					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
//				}
//			}
//		}()
//	}
//	err = client.Connect()
//	if err != nil {
//		log.Printf("Erro ao conectar: %v", err)
//	} else {
//		log.Println("Successfully connected to database and CLIENT")
//		//client.AddEventHandler(func(evt interface{}) {
//		//	switch v := evt.(type) {
//		//	case *events.HistorySync:
//		//		log.Println("Received HistorySync event")
//		//		if client.Store != nil {
//		//			if err := client.Store.PushHistorySync(v.Data); err != nil {
//		//				log.Printf("Failed to push history sync: %v", err)
//		//			} else {
//		//				log.Println("✅ History sync data successfully merged into store")
//		//			}
//		//		}
//		//	case *events.Message:
//		//		// Aqui você pode fazer seu broadcast
//		//		log.Printf("📨 New message from %s: %s", v.Info.Sender.String(), v.Message.GetConversation())
//		//	}
//		//})
//
//		err = client.FetchAppState(ctx, appstate.WAPatchCriticalUnblockLow, true, false)
//		if err != nil {
//			log.Printf("⚠️ Erro ao sincronizar estado do app WAPatchCriticalUnblockLow: %v", err)
//		} else {
//			log.Println("✅ Estado do app sincronizado com sucesso. WAPatchCriticalUnblockLow")
//		}
//		log.Println("<UNK> Estado do app WAPatchCriticalUnblockLow", client.IsConnected())
//
//		err = client.Store.PutCachedSessions(ctx)
//		if err != nil {
//			log.Printf("Erro ao deletar todas as sessões: %v", err)
//		} else {
//			log.Println("Todas as sessões deletadas com sucesso")
//		}
//		//err = client.FetchAppState(ctx, appstate.WAPatchCriticalBlock, true, false)
//		//if err != nil {
//		//	log.Printf("⚠️ Erro ao sincronizar estado do app WAPatchCriticalBlock: %v", err)
//		//} else {
//		//	log.Println("✅ Estado do app sincronizado com sucesso. WAPatchCriticalBlock")
//		//}
//		//err = client.FetchAppState(ctx, appstate.WAPatchRegular, true, false)
//		//if err != nil {
//		//	log.Printf("⚠️ Erro ao sincronizar estado do app WAPatchRegular: %v", err)
//		//} else {
//		//	log.Println("✅ Estado do app sincronizado com sucesso. WAPatchRegular")
//		//}
//		//err = client.FetchAppState(ctx, appstate.WAPatchRegularHigh, true, false)
//		//if err != nil {
//		//	log.Printf("⚠️ Erro ao sincronizar estado do app WAPatchRegularHigh: %v", err)
//		//} else {
//		//	log.Println("✅ Estado do app sincronizado com sucesso. WAPatchRegularHigh")
//		//}
//		//err = client.FetchAppState(ctx, appstate.WAPatchRegularLow, true, false)
//		//if err != nil {
//		//	log.Printf("⚠️ Erro ao sincronizar estado do app appstate.WAPatchRegularLow: %v", err)
//		//} else {
//		//	log.Println("✅ Estado do app sincronizado com sucesso.appstate.WAPatchRegularLow")
//		//}
//		//client.Store.AppStateKeys.GetLatestAppStateSyncKeyID(ctx)
//		//client.Store.PreKeys.Clear()
//		//client.Store.SignedPreKey.Clear()
//		//client.Store.SenderKeys.Clear()
//		//client.Store.Sessions.Clear()
//
//	}
//
//	//if client.Store.ID != nil {
//	//	log.Println("Tentando atualizar devices antes de recriar sessão...")
//	//	_, fetchErr := client.Store.Contacts.GetAllContacts(ctx)
//	//	if fetchErr != nil {
//	//		log.Printf("Erro ao buscar devices: %v", fetchErr)
//	//	} else {
//	//		time.Sleep(2 * time.Second)
//	//		err = client.Connect()
//	//	}
//	//}
//
//	//if err != nil {
//	//	log.Printf("Erro ao conectar cliente: %v", err)
//	//
//	//	// 🔄 Tenta forçar atualização de devices antes de resetar
//	//	if client.Store.ID != nil {
//	//		log.Println("🔁 Tentando atualizar devices antes de recriar sessão...")
//	//		_, fetchErr := client.Store.Contacts.GetAllContacts(ctx)
//	//		if fetchErr != nil {
//	//			log.Printf("Erro ao buscar devices: %v", fetchErr)
//	//		} else {
//	//			// tenta reconectar novamente
//	//			time.Sleep(2 * time.Second)
//	//			err = client.Connect()
//	//		}
//	//	}
//	//}
//
//	// Se ainda falhou após tentativa de atualização → recria sessão
//	if err != nil {
//		log.Println("Falha persistente, recriando sessão e gerando novo QR code...", err)
//
//		_ = os.Remove(tmpFile)
//
//		container2, err2 := sqlstore.New(ctx, "sqlite3", "file:"+tmpFile+"?_foreign_keys=on", dbLog)
//		if err2 != nil {
//			return nil, nil, fmt.Errorf("erro ao recriar sqlstore: %w", err2), tmpFile, ctx
//		}
//		deviceStore, err2 = container2.GetFirstDevice(ctx)
//		if err2 != nil {
//			return nil, nil, fmt.Errorf("erro ao recriar device: %w", err2), tmpFile, ctx
//		}
//
//		client = whatsmeow.NewClient(deviceStore, clientLog)
//		qr, _ = client.GetQRChannel(ctx)
//
//		go func() {
//			for evt := range qr {
//				if evt.Event == "code" {
//					fmt.Println("📱 Novo QR Code (sessão regenerada):")
//					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
//				}
//			}
//		}()
//
//		if err2 = client.Connect(); err2 != nil {
//			return nil, nil, fmt.Errorf("erro ao conectar após recriar sessão: %w", err2), tmpFile, ctx
//		}
//	}
//
//	log.Println("Cliente WhatsApp conectado com sucesso!")
//	return client, qr, nil, tmpFile, ctx
//}
//
//func CloseClient(client *whatsmeow.Client, tmpFile string) error {
//	if client != nil {
//		client.Disconnect()
//	}
//	if tmpFile != "" {
//		if err := os.Remove(tmpFile); err != nil && !os.IsNotExist(err) {
//			log.Printf("Erro ao remover arquivo temporário: %v", err)
//		}
//	}
//	return nil
//}
//
//func SendMessage(ctx context.Context, client *whatsmeow.Client, number string, text string, forward bool) error {
//	jid := types.NewJID(number, types.DefaultUserServer)
//	msg := &waE2E.Message{
//		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
//			Text: proto.String(text),
//			ContextInfo: &waE2E.ContextInfo{
//				IsForwarded: proto.Bool(forward),
//			},
//		},
//	}
//
//	_, err := client.SendMessage(ctx, jid, msg)
//	if err != nil {
//		log.Printf("Erro ao enviar para %s: %v", number, err)
//	}
//	return err
//}
