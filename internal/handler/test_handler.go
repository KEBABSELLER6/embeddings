package handler

import (
	"log/slog"
	"net/http"
)

type TestService interface {
	Test()
}

type TestHandler struct {
	service TestService
}

func NewTestHandler(service TestService) *TestHandler {
	return &TestHandler{service: service}
}

func (h *TestHandler) Test(w http.ResponseWriter, r *http.Request) {
	h.service.Test()
	slog.Info("Hello handler")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
