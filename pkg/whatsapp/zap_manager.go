package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

// Gerencia containers por device, faz proxy das chamadas.
type ZapPkg struct {
	dockerMgr   *DockerManager
	mu          sync.RWMutex
	devices     map[string]*ClientContainer // key: deviceID // VAI SER SO O NUMERO MESMO
	clientImage string                      // imagem do child (ex: "myrepo/whats-child:latest")
}

func NewZapPkg() *ZapPkg {
	dm, err := NewDockerManager()
	if err != nil {
		log.Fatal("Erro ao iniciar docker manager: ", err)
	}

	latestTag, err := dm.GetLatestImageTag("zap-client")
	if err != nil {
		fmt.Printf("[WARN] Não foi possível detectar versão mais recente, usando latest")
		latestTag = "latest"
	}
	imageFull := fmt.Sprintf("%s:%s", "zap-client", latestTag)
	fmt.Println("Docker manager iniciado com imagem: ", imageFull)

	return &ZapPkg{
		dockerMgr:   dm,
		devices:     make(map[string]*ClientContainer),
		clientImage: imageFull,
	}
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
		proxy.ServeHTTP(w, r)
	})

	return mux
}
