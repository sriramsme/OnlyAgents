package httpx

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/sriramsme/OnlyAgents/pkg/logger"
)

type ErrorResponse struct {
	Error string `json:"error"`
	Code  int    `json:"code"`
}

// These helpers live in the api package (not handlers/) because
// they're used across all handler files.

// JSON writes a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(body); err != nil {
		logger.Log.Error("json encode failed", "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(status)

	if _, err := w.Write(buf.Bytes()); err != nil {
		logger.Log.Error("response write failed", "error", err)
	}
}

// Error writes a standard JSON error response.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, ErrorResponse{
		Error: msg,
		Code:  status,
	})
}

// Decode parses a JSON request body into v. Returns false and writes
// a 400 if parsing fails, so handlers can just:
//
//	var req MyRequest
//	if !api.Decode(w, r, &req) { return }
func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	defer func() {
		if err := r.Body.Close(); err != nil {
			logger.Log.Error("request body close failed", "error", err)
		}
	}()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(v); err != nil {
		Error(w, http.StatusBadRequest, "invalid request body")
		return false
	}

	if dec.More() {
		Error(w, http.StatusBadRequest, "invalid request body")
		return false
	}

	return true
}
