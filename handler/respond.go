package handler

// respond.go — shared JSON response helpers used by all handlers.

import (
	"encoding/json"
	"errors"
	"log/slog"
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

// httpError carries an HTTP status alongside an error message. Handler-internal
// functions (validators, multi-write closures inside InTx) return *httpError
// when they have a user-facing error code in mind; the handler unwraps via
// respondHTTPErr at the boundary.
//
// Anything that does NOT implement httpError gets mapped to 500 by
// respondHTTPErr — typed errors are opt-in.
type httpError struct {
	Status int
	Msg    string
}

func (e *httpError) Error() string { return e.Msg }

// httpErr constructs a *httpError. Use this anywhere you want to propagate
// both a status code and a message back to the handler boundary.
func httpErr(status int, msg string) *httpError {
	return &httpError{Status: status, Msg: msg}
}

// respondHTTPErr writes the response from err. If err (or anything in its
// chain via errors.As) is a *httpError, the status and message come from it;
// otherwise the response is 500 with err.Error() as the message.
func respondHTTPErr(w http.ResponseWriter, err error) {
	if he, ok := errors.AsType[*httpError](err); ok {
		respondErr(w, he.Status, he.Msg)
		return
	}
	slog.Error("internal handler error", "err", err)
	respondErr(w, http.StatusInternalServerError, err.Error())
}

// respondInternalErr writes a 500 response with both a user-facing message
// and the underlying error's text, and logs the full error server-side.
// Use at sites that previously swallowed the cause:
//
//	if _, err := q.CreatePlan(...); err != nil {
//	    respondInternalErr(w, "could not create plan", err)
//	    return
//	}
//
// Response body: {"error": "could not create plan: <err.Error()>"}
// Server log: level=ERROR msg="could not create plan" err=<err.Error()>
//
// For 4xx errors (validation, conflict, etc.) keep using respondErr — the
// caller-facing message is the whole point and there is no err to log.
func respondInternalErr(w http.ResponseWriter, msg string, err error) {
	slog.Error(msg, "err", err)
	respondErr(w, http.StatusInternalServerError, msg+": "+err.Error())
}
