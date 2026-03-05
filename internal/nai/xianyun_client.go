package nai

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"grimoire/internal/config"
)

type XianyunClient struct {
	cfg        *config.Manager
	httpClient *http.Client
	logger     *slog.Logger
}

func NewXianyunClient(cfg *config.Manager, logger *slog.Logger) *XianyunClient {
	snapshot := cfg.Snapshot()
	return &XianyunClient{
		cfg:        cfg,
		httpClient: newNAIHTTPClient(snapshot.NAI.Proxy, snapshot.NAI.TimeoutSec, logger),
		logger:     logger,
	}
}

func newNAIHTTPClient(proxyRaw string, timeoutSec int, logger *slog.Logger) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil

	proxyRaw = strings.TrimSpace(proxyRaw)
	if proxyRaw != "" {
		parsed, err := url.Parse(proxyRaw)
		if err != nil {
			if logger != nil {
				logger.Warn("invalid nai proxy url, fallback to direct", "proxy", proxyRaw, "error", err)
			}
		} else {
			transport.Proxy = http.ProxyURL(parsed)
		}
	}

	if timeoutSec <= 0 {
		timeoutSec = 180
	}
	return &http.Client{
		Transport: transport,
		Timeout:   time.Duration(timeoutSec) * time.Second,
	}
}
