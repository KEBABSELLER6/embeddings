package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/KEBABSELLER6/embeddings/internal/config"
	"github.com/KEBABSELLER6/embeddings/internal/handler"
)

func main() {
	cli := &http.Client{Timeout: 30 * time.Second}
	conf, err := config.Configure()
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(1)
	}

	embedder, err := NewEmbedder(conf, cli)
	if err != nil {
		slog.Error("configuration error", "err", err)
		os.Exit(1)
	}

	router := handler.NewRouter(handler.Handlers{
		MaxBodySize:      conf.MaxBodySize,
		EmbeddingHandler: handler.NewEmbeddingHandler(embedder),
	})

	addr := fmt.Sprintf(":%s", conf.Port)
	slog.Info("server starting ", "addr", addr)

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("application stopped", "err", err)
		os.Exit(1)
	}
}
