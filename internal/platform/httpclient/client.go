package httpclient

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func New(timeoutSec int, proxyRaw string, logger *slog.Logger, component string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil

	proxyRaw = strings.TrimSpace(proxyRaw)
	if proxyRaw != "" {
		parsed, err := url.Parse(proxyRaw)
		if err != nil {
			if logger != nil {
				logger.Warn("invalid proxy url, fallback to direct", "component", component, "proxy", proxyRaw, "error", err)
			}
		} else {
			transport.Proxy = http.ProxyURL(parsed)
		}
	}

	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	return &http.Client{
		Transport: transport,
		Timeout:   time.Duration(timeoutSec) * time.Second,
	}
}
