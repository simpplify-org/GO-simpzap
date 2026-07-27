package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/simpplify-org/GO-simpzap/pkg/alerting"
)

// Gerencia containers por device, faz proxy das chamadas.
type ZapPkg struct {
	dockerMgr   *DockerManager
	mu          sync.RWMutex
	devices     map[string]*ClientContainer // key: deviceID // VAI SER SO O NUMERO MESMO
	clientImage string                      // imagem do child (ex: "myrepo/whats-child:latest")

	sentryDSN string // repassado como env SENTRY_DSN para os containers filhos
	sentryEnv string // repassado como env SENTRY_ENVIRONMENT para os containers filhos

	notifiedMu sync.Mutex
	notified   map[string]string // phoneNumber -> último "reason" já reportado ao Sentry, para não duplicar issue a cada tick do health monitor
}

func NewZapPkg() *ZapPkg {
	dm, err := NewDockerManager()
	if err != nil {
		log.Fatal("Erro ao iniciar docker manager: ", err)
	}

	return &ZapPkg{
		dockerMgr:   dm,
		devices:     make(map[string]*ClientContainer),
		clientImage: "zap-client:latest",
		sentryDSN:   os.Getenv("SENTRY_DSN"),
		sentryEnv:   os.Getenv("SENTRY_ENVIRONMENT"),
		notified:    make(map[string]string),
	}
}

// notifyOnce reporta phoneNumber/reason ao Sentry apenas na primeira vez que
// esse motivo é visto (ou depois de resolvido e reincidente), evitando uma
// issue nova a cada tick de 30s do health monitor enquanto o problema
// persiste — o Sentry já agrupa ocorrências repetidas da mesma issue.
func (s *ZapPkg) notifyOnce(phoneNumber, reason, message string) {
	s.notifiedMu.Lock()
	already := s.notified[phoneNumber] == reason
	s.notified[phoneNumber] = reason
	s.notifiedMu.Unlock()

	if already {
		return
	}
	alerting.CaptureSessionDown(phoneNumber, reason, message)
}

// clearNotified esquece o último motivo reportado, para que uma futura
// recorrência do mesmo problema gere um novo alerta.
func (s *ZapPkg) clearNotified(phoneNumber string) {
	s.notifiedMu.Lock()
	delete(s.notified, phoneNumber)
	s.notifiedMu.Unlock()
}

// CreateDevice cria container para device, se já existir retorna o existente.
func (s *ZapPkg) CreateDevice(ctx context.Context, phoneNumber string) (*ClientContainer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if c, ok := s.devices[phoneNumber]; ok {
		return c, nil
	}

	namePrefix := "whats-device-" + sanitizeName(phoneNumber)
	labels := map[string]string{
		"app":          "whatsapp-client",
		"phone_number": phoneNumber,
	}

	existing, err := s.dockerMgr.FindContainerByLabel(ctx, "phone_number", phoneNumber)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar container existente: %w", err)
	}

	if existing != nil {
		log.Printf("[ZapPkg] Reutilizando container existente para %s (ID=%s)", phoneNumber, existing.ID)
		s.devices[phoneNumber] = existing
		return existing, nil
	}

	envs := []string{
		fmt.Sprintf("PHONE_NUMBER=%s", phoneNumber),
		fmt.Sprintf("LOG_LEVEL=info"),
	}
	// Repassa a config do Sentry para o container filho poder reportar a
	// queda de sessão (LoggedOut/StreamReplaced) instantaneamente, assim que
	// o evento acontece — sem depender do polling do health monitor.
	if s.sentryDSN != "" {
		envs = append(envs, fmt.Sprintf("SENTRY_DSN=%s", s.sentryDSN))
	}
	if s.sentryEnv != "" {
		envs = append(envs, fmt.Sprintf("SENTRY_ENVIRONMENT=%s", s.sentryEnv))
	}

	cc, err := s.dockerMgr.StartContainer(ctx, s.clientImage, namePrefix, labels, envs)
	if err != nil {
		return nil, fmt.Errorf("erro ao iniciar container para numero %s: %w", phoneNumber, err)
	}

	// health-check no endpoint do child para garantir start
	if err := s.waitUntilHealthy(cc.Endpoint, 15*time.Second); err != nil {
		_ = s.dockerMgr.StopContainer(ctx, cc.ID)
		_ = s.dockerMgr.RemoveContainer(ctx, cc.ID)
		return nil, fmt.Errorf("container iniciou mas não respondeu: %w", err)
	}

	s.devices[phoneNumber] = cc
	log.Printf("[Service] Device criado: %s -> %s", phoneNumber, cc.Endpoint)
	return cc, nil
}

// RemoveDevice para e remove
func (s *ZapPkg) RemoveDevice(ctx context.Context, deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cc, ok := s.devices[deviceID]
	if !ok {
		return errors.New("device não encontrado")
	}

	if err := s.dockerMgr.StopContainer(ctx, cc.ID); err != nil {
		log.Printf("[Service] falha ao parar container %s: %v", cc.ID, err)
	}
	if err := s.dockerMgr.RemoveContainer(ctx, cc.ID); err != nil {
		log.Printf("[Service] falha ao remover container %s: %v", cc.ID, err)
	}

	delete(s.devices, deviceID)
	log.Printf("[Service] Device %s removido", deviceID)
	return nil
}

// GetDeviceEndpoint retorna endpoint do container do device
func (s *ZapPkg) GetDeviceEndpoint(deviceID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if c, ok := s.devices[deviceID]; ok {
		return c.Endpoint, nil
	}
	return "", errors.New("device não iniciado")
}

// ProxyHandler gera um http.Handler que roteia para o container do device.
// pathPrefix é prefixo usado para extrair device do path, por exemplo "/device/{deviceID}/..."
func (s *ZapPkg) ProxyHandler() http.Handler {
	mux := http.NewServeMux()

	// exemplo: /device/{deviceID}/...
	mux.HandleFunc("/device/", func(w http.ResponseWriter, r *http.Request) {
		// extrai device
		// path esperado: /device/<deviceID>/<rest>
		parts := splitPath(r.URL.Path)
		if len(parts) < 2 {
			http.Error(w, "device não informado", http.StatusBadRequest)
			return
		}
		deviceID := parts[1]

		// garante que o device exista
		endpoint, err := s.GetDeviceEndpoint(deviceID)
		if err != nil {
			ctx := r.Context()
			if _, cerr := s.CreateDevice(ctx, deviceID); cerr != nil {
				http.Error(w, "erro ao criar device: "+cerr.Error(), http.StatusInternalServerError)
				return
			}
			endpoint, _ = s.GetDeviceEndpoint(deviceID)
		}

		target, err := url.Parse(endpoint)
		if err != nil {
			http.Error(w, "endpoint inválido", http.StatusInternalServerError)
			return
		}

		// remap a URL para remover /device/{deviceID} do path antes de repassar
		// ex: /device/123/message/send -> /message/send
		stripPrefix := fmt.Sprintf("/device/%s", deviceID)
		r.URL.Path = singleJoiningSlash("/", r.URL.Path[len(stripPrefix):])

		proxy := httputil.NewSingleHostReverseProxy(target)
		
		// Preservar cabeçalhos de WebSocket que podem ser removidos pelo reverse proxy padrão do Go
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			
			// Se for um upgrade de WebSocket, garante que os cabeçalhos cruciais sejam explicitamente passados ao child
			if r.Header.Get("Upgrade") != "" {
				req.Header.Set("Upgrade", r.Header.Get("Upgrade"))
				req.Header.Set("Connection", "Upgrade")
			}
		}

		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[Proxy Error] falha no proxy para %s: %v", target.String(), err)
			http.Error(w, "Proxy error: "+err.Error(), http.StatusBadGateway)
		}
		
		isWebSocket := r.Header.Get("Upgrade") != ""
		log.Printf("[Proxy] Encaminhando req %s para %s (Path: %s) [WS: %t]", r.Method, target.String(), r.URL.Path, isWebSocket)
		proxy.ServeHTTP(w, r)
	})

	return mux
}

// deviceStatus espelha (só os campos que interessam aqui) o JSON retornado
// por GET /status no container filho (clientservice.ConnectionStatus).
type deviceStatus struct {
	Connected bool   `json:"connected"`
	NeedsQR   bool   `json:"needs_qr"`
	LastEvent string `json:"last_event"`
}

// StartHealthMonitor sobe um loop em background que revalida periodicamente
// os devices registrados. Ele cobre dois casos que a auto-reconexão do
// processo filho (whatsmeow + auto-connect no boot) não resolve sozinha:
//
//  1. Container caiu de vez (ex: crash + RestartPolicy ainda não recriou, ou
//     foi removido por fora) — o cache s.devices fica com um endpoint morto,
//     causando 502 no proxy até uma recriação manual. O monitor limpa essa
//     entrada, e a próxima chamada via ProxyHandler recria o container.
//  2. Sessão foi realmente deslogada (needs_qr) — o único caso que não se
//     autorrecupera de jeito nenhum, precisa de alguém escanear o QR de
//     novo. O monitor loga isso de forma destacada para virar alerta.
func (s *ZapPkg) StartHealthMonitor(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.checkDevicesHealth(ctx)
			}
		}
	}()
	log.Printf("[HealthMonitor] iniciado (intervalo: %s)", interval)
}

func (s *ZapPkg) checkDevicesHealth(ctx context.Context) {
	s.mu.RLock()
	snapshot := make(map[string]*ClientContainer, len(s.devices))
	for phoneNumber, cc := range s.devices {
		snapshot[phoneNumber] = cc
	}
	s.mu.RUnlock()

	for phoneNumber, cc := range snapshot {
		inspect, err := s.dockerMgr.client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: cc.ID})
		if err != nil || !inspect.State.Running {
			log.Printf("[HealthMonitor] ⚠️ container do device %s (ID=%s) não está mais rodando (err=%v) — removendo do cache; será recriado na próxima chamada", phoneNumber, cc.ID, err)
			s.mu.Lock()
			delete(s.devices, phoneNumber)
			s.mu.Unlock()
			// Alerta de backup: se o container morreu antes de conseguir avisar
			// sozinho (ver alerting.CaptureSessionDown no client), o master
			// ainda reporta a queda ao Sentry.
			s.notifyOnce(phoneNumber, "container_down", fmt.Sprintf("container %s parou de responder (err=%v)", cc.ID, err))
			continue
		}

		status, err := fetchDeviceStatus(ctx, cc.Endpoint)
		if err != nil {
			log.Printf("[HealthMonitor] ⚠️ não foi possível checar /status do device %s (%s): %v", phoneNumber, cc.Endpoint, err)
			continue
		}

		switch {
		case status.NeedsQR:
			log.Printf("[HealthMonitor] 🚨 device %s perdeu a sessão (logout) — precisa escanear um novo QR Code em %s/connect/ws", phoneNumber, cc.Endpoint)
			s.notifyOnce(phoneNumber, "needs_qr", fmt.Sprintf("sessão sem login válido — escaneie um novo QR em %s/connect/ws", cc.Endpoint))
		case !status.Connected:
			log.Printf("[HealthMonitor] ⏳ device %s está com sessão válida mas desconectado no momento (last_event=%s) — deve reconectar sozinho", phoneNumber, status.LastEvent)
		default:
			// Saudável de novo: limpa o estado para que uma futura recorrência
			// gere um novo alerta em vez de ficar silenciada para sempre.
			s.clearNotified(phoneNumber)
		}
	}
}

// fetchDeviceStatus consulta GET /status no container filho.
func fetchDeviceStatus(ctx context.Context, endpoint string) (*deviceStatus, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint+"/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var status deviceStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("resposta inválida de %s/status: %w", endpoint, err)
	}
	return &status, nil
}

type DeviceInfo struct {
	ID       string `json:"id"`
	Number   string `json:"number"`
	Endpoint string `json:"endpoint"`
	WsUrl    string `json:"ws_url"`
	Status   string `json:"status"`
}

// ListDevices busca todos os containers no Docker com o label app=whatsapp-client
func (s *ZapPkg) ListDevices(ctx context.Context) ([]DeviceInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	containers, err := s.dockerMgr.client.ListContainers(docker.ListContainersOptions{
		All: true,
		Filters: map[string][]string{
			"label": {"app=whatsapp-client"},
		},
	})
	if err != nil {
		return nil, err
	}

	var list []DeviceInfo
	host := s.dockerMgr.getDockerHost()

	for _, c := range containers {
		phoneNumber := c.Labels["phone_number"]
		if phoneNumber == "" {
			continue
		}

		inspect, err := s.dockerMgr.client.InspectContainerWithOptions(docker.InspectContainerOptions{ID: c.ID})
		if err != nil {
			continue
		}

		port := 0
		if bindings, ok := inspect.NetworkSettings.Ports["8080/tcp"]; ok && len(bindings) > 0 {
			port, _ = strconv.Atoi(bindings[0].HostPort)
		}

		endpoint := fmt.Sprintf("http://%s:%d", host, port)
		wsUrl := "/device/" + phoneNumber + "/connect/ws"

		// Sincroniza o cache interno em memória
		cc := &ClientContainer{
			ID:       c.ID,
			Host:     host,
			Port:     port,
			Endpoint: endpoint,
		}
		s.devices[phoneNumber] = cc

		status := c.State // "running", "exited", etc.

		list = append(list, DeviceInfo{
			ID:       c.ID,
			Number:   phoneNumber,
			Endpoint: endpoint,
			WsUrl:    wsUrl,
			Status:   status,
		})
	}

	return list, nil
}
