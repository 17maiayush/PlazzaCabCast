package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"ridehailing/internal/domain"
)

// endpoint is a handler that returns what to send instead of writing it, so every response
// and every error goes through the same two functions.
type endpoint func(r *http.Request) (status int, body any, err error)

func (h *handler) handle(fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, body, err := fn(r)
		if err != nil {
			h.writeError(w, err)
			return
		}
		h.writeJSON(w, status, body)
	}
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: invalid JSON body: %w", domain.ErrValidation, err)
	}
	return nil
}

func (h *handler) writeJSON(w http.ResponseWriter, status int, body any) {
	if body == nil {
		w.WriteHeader(status)
		return
	}
	raw, err := json.Marshal(body)
	if err != nil {
		h.writeError(w, fmt.Errorf("marshal response: %w", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (h *handler) writeError(w http.ResponseWriter, err error) {
	status, code := statusFor(err)
	message := err.Error()
	if status == http.StatusInternalServerError {
		h.logger.Error("request failed", "err", err)
		message = "internal error"
	}
	h.writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}

func statusFor(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		return http.StatusBadRequest, "VALIDATION_ERROR"
	case errors.Is(err, domain.ErrInvalidCoupon):
		return http.StatusBadRequest, "INVALID_COUPON"
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, domain.ErrAlreadyExists):
		return http.StatusConflict, "ALREADY_EXISTS"
	case errors.Is(err, domain.ErrNoDriverAvailable):
		return http.StatusConflict, "NO_DRIVER_AVAILABLE"
	case errors.Is(err, domain.ErrInvalidTransition):
		return http.StatusConflict, "INVALID_STATE"
	case errors.Is(err, domain.ErrRequestInFlight):
		return http.StatusConflict, "REQUEST_IN_PROGRESS"
	case errors.Is(err, domain.ErrIdempotencyMismatch):
		return http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED"
	}
	return http.StatusInternalServerError, "INTERNAL"
}
