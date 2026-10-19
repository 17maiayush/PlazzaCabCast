package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"ridehailing/internal/domain"
)

// idempotent replays the stored response for a repeated Idempotency-Key. Both successes and
// 4xx outcomes are stored: a client that wants a fresh attempt sends a fresh key.
func (h *handler) idempotent(store IdempotencyStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			h.writeError(w, fmt.Errorf("%w: reading body: %w", domain.ErrValidation, err))
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		scope := scopeFor(body, key)
		sum := sha256.Sum256(body)
		replay, err := store.Begin(scope, hex.EncodeToString(sum[:]))
		if err != nil {
			h.writeError(w, err)
			return
		}
		if replay != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Idempotent-Replayed", "true")
			w.WriteHeader(replay.Status)
			_, _ = w.Write(replay.Body)
			return
		}

		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		committed := false
		defer func() {
			if !committed {
				store.Abort(scope) // a panic must not leave the key claimed forever
			}
		}()
		next.ServeHTTP(rec, r)
		store.Commit(scope, domain.IdempotentResponse{Status: rec.status, Body: rec.body.Bytes()})
		committed = true
	})
}

// scopeFor keys the claim by user so two riders can reuse the same client-generated key.
// A body too broken to name a user is claimed under the bare key; the handler rejects it anyway.
func scopeFor(body []byte, key string) string {
	var peek struct {
		UserID string `json:"user_id"`
	}
	_ = json.Unmarshal(body, &peek)
	return peek.UserID + ":" + key
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
