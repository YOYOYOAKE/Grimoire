package nai

import (
	"log/slog"
	"net/http"
	"time"

	"grimoire/internal/config"
)

const defaultHTTPTimeout = 60 * time.Second

type XianyunClient struct {
	cfg        *config.Manager
	httpClient *http.Client
	logger     *slog.Logger
}

func NewXianyunClient(cfg *config.Manager, logger *slog.Logger) *XianyunClient {
	return &XianyunClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
		logger:     logger,
	}
}
