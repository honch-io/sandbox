package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

type Mode string

const (
	ModeOnline      Mode = "online"
	ModeOffline     Mode = "offline"
	ModeServerError Mode = "server-error"
)

func ParseMode(value string) (Mode, error) {
	switch Mode(value) {
	case ModeOnline, ModeOffline, ModeServerError:
		return Mode(value), nil
	default:
		return "", fmt.Errorf("unsupported network mode %q", value)
	}
}

func (m Mode) String() string {
	return string(m)
}

type Controller struct {
	mu   sync.RWMutex
	mode Mode
}

func NewController(mode Mode) *Controller {
	if mode == "" {
		mode = ModeOnline
	}
	return &Controller{mode: mode}
}

func (c *Controller) SetMode(mode Mode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mode = mode
}

func (c *Controller) Mode() Mode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mode
}

func (c *Controller) Handler(target string) (http.Handler, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	rp := httputil.NewSingleHostReverseProxy(u)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch c.Mode() {
		case ModeOffline:
			http.Error(w, "sandbox proxy offline", http.StatusServiceUnavailable)
		case ModeServerError:
			http.Error(w, "sandbox injected server error", http.StatusInternalServerError)
		default:
			rp.ServeHTTP(w, r)
		}
	}), nil
}

func WriteMode(w io.Writer, mode Mode) error {
	_, err := fmt.Fprintf(w, "network mode: %s\n", mode)
	return err
}
