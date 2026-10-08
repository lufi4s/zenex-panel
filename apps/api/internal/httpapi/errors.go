package httpapi

import (
	"encoding/json"
	"net/http"
)

// APIError is the structured error body returned to clients.
// Messages are safe to show to users; internal details go to logs only.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

var (
	ErrNotFound         = APIError{Status: http.StatusNotFound, Code: "not_found", Message: "resource not found"}
	ErrMethodNotAllowed = APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "method not allowed"}
	ErrInternal         = APIError{Status: http.StatusInternalServerError, Code: "internal_error", Message: "internal server error"}
	ErrBodyTooLarge     = APIError{Status: http.StatusRequestEntityTooLarge, Code: "body_too_large", Message: "request body too large"}
)

func writeError(w http.ResponseWriter, requestID string, e APIError) {
	var env errorEnvelope
	env.Error.Code = e.Code
	env.Error.Message = e.Message
	env.Error.RequestID = requestID

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(env)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
