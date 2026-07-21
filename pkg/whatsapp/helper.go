package whatsapp

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

// splitPath("/device/123/message/send") => ["device","123","message","send"]
func splitPath(p string) []string {
	s := strings.Trim(p, "/")
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "/")
}

func sanitizeName(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ":", "-")
	return s
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func (s *ZapPkg) waitUntilHealthy(endpoint string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(endpoint + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("health check falhou")
}

func getFreePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}
