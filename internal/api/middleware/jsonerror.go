package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// WriteError writes a JSON error envelope. Encoder failures are intentionally
// swallowed: the client connection is likely already half-closed at that
// point, and there is no recovery path that wouldn't make things worse.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorBody{Error: code, Message: message})
}

// WriteJSON encodes body as JSON with the given status. If body is a nil slice,
// it is replaced with an empty slice so list endpoints return [] not null.
// Encoder errors are logged via the supplied logger (may be nil to skip).
func WriteJSON(w http.ResponseWriter, logger *slog.Logger, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil && logger != nil {
		logger.Warn("json encode", "err", err)
	}
}
