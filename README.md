# Ride Hailing Backend

A backend for an Uber-style ride hailing service, built for Plazza's SDE-1 machine-coding round. Go 1.24+, standard library only, in-memory storage, JSON REST API. Everything in the mandatory scope is implemented, plus all four bonus items: demand/supply surge, a switchable matching strategy, cancellation with a fee policy, and race-safe booking.

```
make run      # API on :8080 (PORT, MATCHING, SURGE, MATCH_RADIUS_KM, CANCEL_GRACE_MIN, CANCEL_FEE_INR)
make demo     # curl walkthrough of every endpoint and edge case against the running server
make e2e      # headless-Chrome walkthrough of the console (needs Node and Chrome)
make test     # go test -race ./...
make cover    # same, with a gate that fails below 100% statement coverage
make fuzz     # random-input search for fare invariant violations (FUZZTIME=20s)
make lint     # gofmt, go vet, golangci-lint when installed
make check    # lint + cover
```

Current state: `make check` passes with 100.0% coverage, 0 lint issues, every test under the race detector.

## Repository layout

```
.github/workflows/   CI: lint, vet, race tests, coverage gate
cmd/server/          entry point and composition root
internal/            domain, service, api, pricing, matching, cancellation, store, web
configs/             .env.example with every setting and its default
docs/                API reference and the design spec
scripts/             demo.sh, the curl walkthrough
test/e2e/            browser walkthrough of the console
ARCHITECTURE.md      layering, flows, concurrency, how to extend
DECISIONS.md         trade-offs and what replaces each in production
Dockerfile           static binary on distroless, non-root
```

## Console

After `make run`, open http://localhost:8080/. It is one page embedded in the binary (`internal/web`), served by the same process as the API, so there is no second server, no build step and no CORS. It drives every endpoint: register riders and drivers, move cabs, add and delete coupons, book with an optional Idempotency-Key, end or cancel rides with a fare breakdown, and fetch history. A map of Bengaluru shows drivers, ongoing pickups and the 5 km matching radius; clicking it drops a pin usable as any location. "Load demo data" sets up the same scenario as `scripts/demo.sh`. Every call and its JSON response is listed under Activity.

The page reads everything from the server and refreshes every five seconds, so a reload or a parallel `make demo` shows up immediately. The trip planner shows a live quote from `POST /rides/estimate` as you change the pickup, drop, car type or coupon: price, breakdown, and whether a driver, or only an upgrade, is in range.

## What it does

| Requirement | Where |
|---|---|
| Register a user / a driver | `POST /users`, `POST /drivers` |
| Update a cab's location | `PUT /drivers/{id}/location` |
| Book a ride if a driver is within the radius | `POST /rides`, radius from `MATCH_RADIUS_KM` (5 km) |
| End a ride and get its cost | `POST /rides/{id}/end` returns the full fare breakdown |
| Tiered pricing, minimum fare, per-car-type rates | `internal/pricing` |
| Hatchback unavailable -> free upgrade to Sedan | booking tries the requested type first, then the upgrade path; the fare is keyed on the requested type |
| Coupons: add, delete, apply at booking | `POST /coupons`, `DELETE /coupons/{code}`, `coupon_code` on booking |
| Ride history for a user / a driver | `GET /users/{id}/rides`, `GET /drivers/{id}/rides`, `?status=` filter |
| Surge pricing (bonus) | `pricing.StepSurge`, on by default, `SURGE=off` to disable |
| Configurable matching (bonus) | `MATCHING=nearest` or `rated`, resolved in `cmd/server/wiring.go` |
| Cancellation with a fee policy (bonus) | `POST /rides/{id}/cancel`, `cancellation.GracePolicy` |
| Concurrency safety (bonus) | atomic driver reservation in the store, proven by goroutine tests under `-race` |
| Fare estimate (extra) | `POST /rides/estimate` quotes price, surge, coupon and upgrade before booking, without reserving a driver |
| List endpoints (extra) | `GET /users`, `GET /drivers`, `GET /coupons`; the console is fully server-backed |
| Idempotent booking (extra) | `Idempotency-Key` header on `POST /rides` replays the stored response |

## API

All bodies are JSON. Money is integer paise in `*_paise` fields; totals also carry a readable `total_inr`.

```
GET    /users                     list riders
GET    /drivers                   list drivers with live status and location
POST   /users                     {name}
POST   /drivers                   {name, car_type, location{lat,lng}, rating?}        rating defaults to 5.0
PUT    /drivers/{id}/location     {lat, lng}
POST   /rides                     {user_id, car_type, pickup{lat,lng}, coupon_code?}  header Idempotency-Key optional
POST   /rides/estimate            {car_type, pickup, drop, coupon_code?}            quote only, reserves nothing
POST   /rides/{id}/end            {drop{lat,lng}}
POST   /rides/{id}/cancel
GET    /users/{id}/rides?status=  ongoing | completed | cancelled (optional, case-insensitive)
GET    /drivers/{id}/rides?status=
GET    /coupons                   list coupons
POST   /coupons                   {code, kind: PERCENT|FLAT, percent | amount_paise, expires_at?}
DELETE /coupons/{code}
GET    /healthz
```

The error table, status codes and a full response sample are in [docs/API.md](docs/API.md).

## Assumptions

1. A driver and their cab are one entity. "Update the cab's location" updates the driver.
2. Booking assigns a driver immediately and the ride is `ONGOING`; there is no accept/reject step. The state machine is `ONGOING -> COMPLETED | CANCELLED`, nothing else.
3. Pickup is known at booking, drop at the end. Distance is the great-circle (haversine) distance between them, rounded once to whole metres. No road routing.
4. Pricing tiers are marginal, like tax brackets. "First 2 km", "3-5 km", "6 km and beyond" are read as the continuous bands [0, 2 km], (2, 5 km], (5 km, ∞). A 7 km Hatchback ride is 2x10 + 3x8 + 2x5 = Rs 54. A consequence worth knowing: up to about 6.2 km the metered Hatchback fare is below Rs 50, so the minimum fare applies.
5. Hatchback uses the tariff from the brief (minimum Rs 50; Rs 10 / 8 / 5 per km). Sedan is my number: minimum Rs 80; Rs 15 / 12 / 8 per km.
6. The fare is computed from the car type the rider *requested*, never the one assigned. That single rule is what makes the upgrade free; there is no special case anywhere.
7. Surge multiplier and coupon are snapshotted at booking and applied at the end. The rider sees a deterministic price even if the coupon is deleted or demand changes mid-ride.
8. An invalid, deleted or expired coupon rejects the booking (400) rather than silently booking without the discount. Percent coupons take 1-100; flat coupons take a positive amount in paise. A discount never takes the total below zero.
9. The matching radius is 5 km by default. An available driver of the requested type always beats an upgrade, even if the upgrade is nearer.
10. A new driver starts at a 5.0 rating unless one is given. Ratings never change; there is no rating flow in scope.
11. Cancellation is free within 3 minutes of booking, then a flat Rs 20. Both are configurable.
12. Money is integer paise, distance is integer metres, surge is an integer percent (100 = 1.00x). No floating point ever touches money. Rounding is half-up and happens exactly where the spec says.
13. Idempotency keys are scoped to the requesting user and live forever in memory. A real deployment would expire them after 24 hours.

## Design

Layered packages with one rule: dependencies point inward. `domain` imports nothing, `service` owns the interfaces it needs, and the adapters (`store`, `pricing`, `matching`, `cancellation`) import only `domain`. Concrete types meet in one file, `cmd/server/wiring.go`.

Key decisions:

1. The fare is computed from the requested car type, so the free upgrade needs no special case.
2. Surge and coupon are snapshotted at booking, so the end-of-ride price is deterministic.
3. Money is integer paise and distance integer metres; rounding is half-up at fixed points.
4. Two bookings for one driver collide at a single atomic compare-and-set in the store; bookings retry down the ranked list.
5. A driver is released only after the ride's transition is saved, and any failure after a reservation releases it again.

Full design, concurrency model, idempotency and an extension guide: [ARCHITECTURE.md](ARCHITECTURE.md). Trade-offs and what replaces each in production: [DECISIONS.md](DECISIONS.md).

## Testing

```
make check
```

runs `gofmt`, `go vet`, `golangci-lint` (if installed) and `go test -race` with a coverage profile over every package, then fails if the total is below 100%. Coverage includes `cmd/server`: `main` is a thin shell with an injectable `exit`, and `run` is started on port 0 and stopped through its context.

What the tests prove, by package:

| Package | Proves |
|---|---|
| `domain` | haversine against exact reference distances; every state transition from every state; coupon expiry at the boundary; deep copies really are independent |
| `pricing` | the tier table at 0, 1, exactly 2, 2.001, exactly 5, 6, 7 and 12.345 km; minimum fare; Sedan vs Hatchback; surge rounding half-up at the paisa; percent and flat coupons, including a flat coupon larger than the fare; every way a tariff table can be malformed; the surge step table including zero supply; a fuzz test checks on random inputs that every step only moves one way, the discount never exceeds the fare, and one more metre never costs less |
| `store` | reserve succeeds exactly once (also from 50 goroutines); radius filtering is inclusive; `Update` is atomic and a failed mutation leaves the row untouched; returned rides never alias stored ones; idempotency claim, replay, mismatch, in-flight, abort |
| `service` | nearest driver of the requested type wins over a nearer upgrade; upgrade is priced as the requested type; coupon snapshot survives deletion; expired coupon rejected before any reservation; ending twice leaves a reassigned driver alone; cancellation fee inside and after the grace window with a fake clock; surge locked at booking; both strategies swappable; driver released on save failure and on panic; 50 goroutines / 1 driver and 20 goroutines / 10 drivers |
| `api` | every endpoint's happy path and every row of the error table; byte-identical idempotent replay; malformed JSON rejected everywhere; oversized bodies rejected |
| `web` | the embedded page is served as HTML at `/` |
| `cmd/server` | every config variable, valid and invalid; every strategy combination wires up; the console is mounted at `/`; the server starts, answers and shuts down cleanly; `main` exits 1 on bad config |

`make demo` is the manual counterpart: it walks the running server through every endpoint and prints the edge cases from the brief one after another.

## With more time

- Persistence: Postgres with the reservation as a conditional `UPDATE` and ride transitions under `SELECT ... FOR UPDATE`; a transactional outbox for ride events.
- Driver lifecycle: `OFFLINE`, accept/reject with a timeout, `ARRIVING`, and a rider-facing ETA.
- Real distances and durations from a routing service, with a time component in the tariff.
- Rating updates after a ride, which would make `HighestRated` meaningful over time.
- Idempotency key TTL, pagination on history, request logging with request IDs, metrics.
- An OpenAPI document generated from the DTOs, and a small CLI client over the same service layer.
- Geo indexing (a grid or geohash) once the driver count makes a linear scan noticeable.

## How I used AI

<!-- Ayush: this section is yours to finish. The structure is here; fill in the examples in your own words. -->

**Tools.** Claude Code (Claude Fable 5.1) inside VS Code, for the whole exercise.

**What I prompted for.**

- _[Ayush: how you briefed it: the PDF, the company context, the bar you set, the rules you gave (no commits by the AI, 100% coverage, human-style comments)]_
- _[Ayush: the design phase: language and interface choice, the HLD reviewed section by section before any code]_
- _[Ayush: the implementation phase: test-first per package, gates after every package]_

**What I rejected or rewrote, and why.**

- _[Ayush: example 1]_
- _[Ayush: example 2]_
- _[Ayush: example 3]_

<!--
Ayush: corrections you made during this session that you may want to describe above:
- the first HLD read surge after the reservation; you moved it before, so supply still counts the driver about to be taken
- the first diagram had adapters importing the service; you had it fixed so adapters import only domain and implement service-defined ports
- the deferred release was inside the ranking loop; you had it moved into reserveAndSave so each attempt compensates immediately
- End/Cancel released the driver before checking the transition; you required release only after a successful atomic update, never on INVALID_STATE
- StepSurge used integer division; you asked for demand <= k * supply so zero supply needs no special case
- idempotency with replay of the stored response was your addition, not in the brief
- sync.WaitGroup.Go needed Go 1.25; replaced with Add/Done to keep the module on 1.24
- several AI-written test helpers carried parameters that never varied; the linter caught them and they were removed
-->

**What I own.** Every line was read before it was kept. The design decisions in this README are mine; the AI typed most of the code against tests I had agreed to first.
