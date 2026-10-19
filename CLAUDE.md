# Ride Hailing Backend

Go take-home for Plazza's SDE-1 round: stdlib-only REST API over an in-memory store. The design spec at `docs/specs/2026-10-19-ride-hailing-design.md` is the source of truth; keep this file in sync with it.

## Layout and dependency rule

```
cmd/server/             composition root: env -> config, wires adapters into services, serves HTTP
internal/domain/        pure types, sentinel errors, ride state machine. Imports nothing from internal/
internal/service/       use cases. Defines the repo and strategy ports it consumes
internal/api/           HTTP adapter: routes, DTOs, validation, error-to-status mapping, idempotency
internal/pricing/       tariffs, marginal tiers, minimum fare, coupon discount, surge strategies
internal/matching/      driver ranking strategies
internal/cancellation/  cancellation fee policies
internal/store/         in-memory repos: one RWMutex per repo, atomic Reserve/Release/Update
internal/web/           embedded single-page console served at GET /; talks to the API only over HTTP
configs/                .env.example
scripts/demo.sh         curl walkthrough
test/e2e/               headless-Chrome console walkthrough (Node, optional)
docs/                   API.md and the design spec
```

Arrows point inward: `api -> service -> domain`. Adapters (`store`, `pricing`, `matching`, `cancellation`) import only `domain` and satisfy the ports structurally. Concrete types meet only in `cmd/server`. Never import `service` from an adapter, never import one adapter from another.

## Conventions

- Money is `domain.Paise` (int64), distance is `domain.Metres` (int64), surge is `domain.Multiplier` (percent, 100 = 1.00x). Floats never touch money; the haversine result is rounded once.
- Sentinel errors live in `domain`. Wrap with `fmt.Errorf("...: %w", err)`; the HTTP layer maps them with `errors.Is`.
- Never hold two repo locks at once. Driver reservation is a compare-and-set in the store; services are optimistic and try the next ranked candidate.
- Fare is computed from the requested car type. Surge and coupon are snapshotted at booking.
- Release a driver only after the ride transition has been saved. On `ErrInvalidTransition` never touch the driver.
- HTTP handlers are `endpoint` functions returning `(status, body, err)`; one `handle` wrapper writes them, so no handler touches the ResponseWriter.
- Tests are table-driven. Every error branch is covered; the gate is 100% (`make cover`).
- No third-party dependencies.

## Comments

Only where the why is not obvious. One line, plain voice. No doc comments that restate the identifier.

## Commands

```
make test    go test -race ./...
make cover   coverage gate, must print 100.0%
make fuzz    fare invariants on random inputs (FUZZTIME=20s)
make lint    gofmt, go vet, golangci-lint if installed
make check   lint + cover
make run     API + console on $PORT (default 8080), reads .env
make e2e     browser walkthrough against a running server
```

## Workflow

Work lands one commit-sized unit at a time, in the order of the spec's implementation plan. The author reviews and commits every unit; never run `git commit`.
