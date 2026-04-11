// respond.go — shared JSON response helpers used by all handlers.
package handler

import (
	"encoding/json"
	"net/http"
)

// respond writes a JSON response with the given status code.
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic("failed to write JSON response: " + err.Error()) // should never happen
	}
}

// respondErr writes a JSON error response: {"error": "message"}.
func respondErr(w http.ResponseWriter, status int, msg string) {
	respond(w, status, map[string]string{"error": msg})
}
