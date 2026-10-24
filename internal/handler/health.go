package handler

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
)

type HealthResponse struct {
	Status string `json:"status"`
}

func HandleHealthRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := HealthResponse{Status: "ok"}
	out, err := json.Marshal(response)
	if err != nil {
		HandleError(w, r, fmt.Errorf("internal error: %v", err))
		return
	}
	w.Write(out)
}
