package nai

import (
	"log/slog"
	"net/http"

	"grimoire/internal/config"
)

type XianyunClient struct {
	cfg        *config.Manager
	httpClient *http.Client
	logger     *slog.Logger
}

func NewXianyunClient(cfg *config.Manager, logger *slog.Logger) *XianyunClient {
	return &XianyunClient{
		cfg:        cfg,
		httpClient: &http.Client{},
		logger:     logger,
	}
}
