package store

import (
	"bytes"
	"fmt"
	"sync"

	"ridehailing/internal/domain"
)

type idempotencyEntry struct {
	fingerprint string
	response    *domain.IdempotentResponse // nil while the first request is still running
}

type Idempotency struct {
	mu      sync.Mutex
	entries map[string]*idempotencyEntry
}

func NewIdempotency() *Idempotency { return &Idempotency{entries: map[string]*idempotencyEntry{}} }

// Begin claims the scope for a new request, or returns the stored response to replay.
// A different fingerprint under the same scope is a client bug and is rejected rather than replayed.
func (s *Idempotency) Begin(scope, fingerprint string) (*domain.IdempotentResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[scope]
	if !ok {
		s.entries[scope] = &idempotencyEntry{fingerprint: fingerprint}
		return nil, nil
	}
	if e.fingerprint != fingerprint {
		return nil, fmt.Errorf("%w: %s", domain.ErrIdempotencyMismatch, scope)
	}
	if e.response == nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrRequestInFlight, scope)
	}
	return &domain.IdempotentResponse{Status: e.response.Status, Body: bytes.Clone(e.response.Body)}, nil
}

// Commit after an Abort is a no-op: the claim is gone, so there is nothing to attach the response to.
func (s *Idempotency) Commit(scope string, resp domain.IdempotentResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[scope]; ok {
		e.response = &domain.IdempotentResponse{Status: resp.Status, Body: bytes.Clone(resp.Body)}
	}
}

// Abort drops an in-flight claim so the client can retry; a committed response is kept.
func (s *Idempotency) Abort(scope string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[scope]; ok && e.response == nil {
		delete(s.entries, scope)
	}
}
