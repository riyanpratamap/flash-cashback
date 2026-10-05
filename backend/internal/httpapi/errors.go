package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/riyanpratamap/flash-cashback/backend/internal/domain"
	"github.com/riyanpratamap/flash-cashback/backend/internal/store"
)

// Error codes of the contract's Errors table.
const (
	codeMissingUser      = "MISSING_USER"
	codeInvalidUser      = "INVALID_USER"
	codeMissingKey       = "MISSING_IDEMPOTENCY_KEY"
	codeInvalidKey       = "INVALID_IDEMPOTENCY_KEY"
	codeMalformed        = "MALFORMED_REQUEST"
	codeInvalidAmount    = "INVALID_AMOUNT"
	codeNotFound         = "NOT_FOUND"
	codeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	codeInternal         = "INTERNAL_ERROR"
	codeServiceBusy      = "SERVICE_BUSY"
)

// Messages are fixed text per code: for logs, never for users, and never a
// driver error, SQL, or stack trace.
var messages = map[string]string{
	codeMissingUser:      "X-User-ID header is required",
	codeInvalidUser:      "X-User-ID header is not valid",
	codeMissingKey:       "Idempotency-Key header is required",
	codeInvalidKey:       "Idempotency-Key header is not a canonical UUID",
	codeMalformed:        "request cannot be read",
	codeInvalidAmount:    "amount must be a whole number from 1 to 10000000",
	codeNotFound:         "no such route",
	codeMethodNotAllowed: "method not allowed",
	codeInternal:         "internal error",
	codeServiceBusy:      "A lock wait timed out; nothing was committed; retry with the same key",
}

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// mapError is the one place an error becomes an HTTP status and code.
// Anything unknown is 500 INTERNAL_ERROR.
func mapError(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrMissingUser):
		return http.StatusBadRequest, codeMissingUser
	case errors.Is(err, domain.ErrInvalidUser):
		return http.StatusBadRequest, codeInvalidUser
	case errors.Is(err, domain.ErrMissingIdempotencyKey):
		return http.StatusBadRequest, codeMissingKey
	case errors.Is(err, domain.ErrInvalidIdempotencyKey):
		return http.StatusBadRequest, codeInvalidKey
	case errors.Is(err, domain.ErrMalformedRequest):
		return http.StatusBadRequest, codeMalformed
	case errors.Is(err, domain.ErrInvalidAmount):
		return http.StatusUnprocessableEntity, codeInvalidAmount
	case errors.Is(err, store.ErrBusy):
		return http.StatusServiceUnavailable, codeServiceBusy
	default: // includes store.ErrInvariant and store.ErrUnknownOutcome
		return http.StatusInternalServerError, codeInternal
	}
}

// writeFailure maps err and writes the D21 envelope. An unmapped error is
// logged once, by type only: driver text may carry SQL or values.
func (d Deps) writeFailure(w http.ResponseWriter, r *http.Request, err error) {
	status, code := mapError(err)
	switch {
	case errors.Is(err, store.ErrDeadlock): // 503 to the client, but a lock-order bug to us
		d.logger().Error("request failed", "event", "deadlock", "request_id", requestIDFrom(r.Context()), "error_type", fmt.Sprintf("%T", err))
	case errors.Is(err, store.ErrInvariant):
		d.logger().Error("request failed", "event", "invariant_violation", "request_id", requestIDFrom(r.Context()), "error_type", fmt.Sprintf("%T", err))
	case status == http.StatusInternalServerError:
		d.logger().Error("request failed", "request_id", requestIDFrom(r.Context()), "error_type", fmt.Sprintf("%T", err))
	}
	writeError(w, r, status, code)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: errorDetail{
		Code:      code,
		Message:   messages[code],
		RequestID: requestIDFrom(r.Context()),
	}}) // the status is sent; a failed write has nowhere to be reported
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusNotFound, codeNotFound)
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, http.StatusMethodNotAllowed, codeMethodNotAllowed)
}
