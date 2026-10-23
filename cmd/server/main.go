package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/KEBABSELLER6/embeddings/internal/handler"
	"github.com/KEBABSELLER6/embeddings/internal/service"
)

func main() {
	testService := service.NewTestService()
	testHandler := handler.NewTestHandler(testService)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /test", testHandler.Test)

	slog.Info("server starting ", "addr", ":8080")
	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("application stopped", "err", err)
		os.Exit(1)
	}
}
