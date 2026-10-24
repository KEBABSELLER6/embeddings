package handler

import (
	"log/slog"
	"net/http"
)

type Handlers struct {
	MaxBodySize      int64
	EmbeddingHandler *EmbeddingHandler
	Logger           *slog.Logger
}

func NewRouter(h Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", HandleHealthRequest)
	mux.HandleFunc("POST /api/v1/embed", h.EmbeddingHandler.HandleEmbeddingRequest)

	logging := NewLoggingHandler(h.Logger)
	maxSize := NewMaxSizeHandler(h.MaxBodySize)
	return http.HandlerFunc(CorrelationID(logging.Logging(maxSize.LimitBodySize(mux.ServeHTTP))))
}
