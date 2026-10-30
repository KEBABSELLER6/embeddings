package handler

import (
	"log/slog"
	"net/http"
)

type Handlers struct {
	MaxBodySize      int64
	EmbeddingHandler *EmbeddingHandler
	Logger           *slog.Logger
	MetricsHandler   *MetricsHandler
	Auth             *APIKeyAuthHandler
}

func NewRouter(h Handlers) http.Handler {
	if h.Auth == nil {
		panic("http: APIKeyAuthHandler is required")
	}

	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/embed", h.Auth.Authenticate(http.HandlerFunc(h.EmbeddingHandler.HandleEmbeddingRequest)))

	logging := NewLoggingHandler(h.Logger)
	maxSize := NewMaxSizeHandler(h.MaxBodySize)

	var router http.Handler = h.MetricsHandler.Metrics(mux)
	router = logging.Logging(router)
	router = CorrelationID(router)
	router = maxSize.LimitBodySize(router)
	return router
}
