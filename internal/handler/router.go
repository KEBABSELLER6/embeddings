package handler

import (
	"log/slog"
	"net/http"
)

type Handlers struct {
	MaxBodySize      int64
	EmbeddingHandler *EmbeddingHandler
}

func NewRouter(h Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", HandleHealthRequest)
	mux.HandleFunc("POST /api/v1/embed", h.EmbeddingHandler.HandleEmbeddingRequest)

	logging := NewLoggingHandler(slog.Default())
	maxSize := NewMaxSizeHandler(h.MaxBodySize)
	return http.HandlerFunc(CorrelationID(logging.Logging(maxSize.LimitBodySize(mux.ServeHTTP))))
}
