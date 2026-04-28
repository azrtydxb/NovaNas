package middleware

import (
	"encoding/json"
	"net/http"
)

type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorBody{Error: code, Message: message})
}
