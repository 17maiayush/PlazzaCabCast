# Decisions

Each entry is a choice made for this exercise, what it costs, and what would replace it in production.

## 1. In-memory store with per-repo locks

instead of a database. Simple, fast, and the compare-and-set reservation is honest about where the race is. With Postgres the same shape becomes `UPDATE drivers SET status='BUSY' WHERE id=$1 AND status='AVAILABLE'` and the ride transition becomes a row lock; the service code would not change.

## 2. Standard library only

`net/http` with Go 1.22 method routing, `log/slog`, `crypto/rand`. No framework to explain, nothing to `go get`, and the whole binary is testable from `main` down.

## 3. Haversine, not routing

A straight-line distance is wrong by the road factor, but it is deterministic, dependency-free and good enough to demonstrate the pricing engine. The distance function is one method on `Location`; a routing client would replace it.

## 4. Surge and coupon locked at booking

Predictable for the rider, simpler to reason about, slightly less revenue-optimal than re-pricing at the end.

## 5. Strict JSON

(`DisallowUnknownFields`, pointer coordinates). A typo in a field name is a 400, not a silently ignored field or a ride at latitude 0.

## 6. Handlers return `(status, body, err)`

and one wrapper writes them. Slightly unusual, but it means no handler touches the `ResponseWriter` and the error-to-status table exists exactly once.
