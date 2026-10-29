package handler

import (
	"log/slog"
	"net/http"

	"github.com/KEBABSELLER6/embeddings/internal/common"
)

type Handlers struct {
	MaxBodySize      int64
	EmbeddingHandler *EmbeddingHandler
	Logger           *slog.Logger
	MetricsHandler   *MetricsHandler
}

func NewRouter(h Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(common.HealthzPattern, HandleHealthRequest)
	mux.HandleFunc("POST /api/v1/embed", h.EmbeddingHandler.HandleEmbeddingRequest)

	logging := NewLoggingHandler(h.Logger)
	maxSize := NewMaxSizeHandler(h.MaxBodySize)

	var router http.Handler = h.MetricsHandler.Metrics(mux)
	router = logging.Logging(router)
	router = CorrelationID(router)
	router = maxSize.LimitBodySize(router)
	return router
}
