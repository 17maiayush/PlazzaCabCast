package domain

import "errors"

var (
	ErrRequestInFlight     = errors.New("a request with this idempotency key is still in progress")
	ErrIdempotencyMismatch = errors.New("idempotency key reused with a different request body")
)

// IdempotentResponse is what gets replayed when a client repeats an Idempotency-Key.
type IdempotentResponse struct {
	Status int
	Body   []byte
}
