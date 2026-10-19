# Ride Hailing Backend: Design Spec

Date: 2026-10-19. Status: approved, implementation in progress.

## 1. Goal and scope

A backend for an Uber-style ride hailing platform, built for Plazza's SDE-1 machine-coding round. Go 1.24+, standard library only, in-memory storage, JSON REST API.

Mandatory: register user, register driver, update cab location, book a ride when a driver is within a radius, end a ride and get its cost, tiered pricing, ride history for users and drivers, Sedan and Hatchback with different rates and a free Hatchback-to-Sedan upgrade, coupons (add, delete, apply at booking).

Bonus, all four: pluggable surge multiplier, switchable driver-matching strategy, cancellation with a fee policy, concurrency safety when two users book the same driver.

## 2. Assumptions

1. A driver and their cab are one entity. "Update cab location" updates the driver's location.
2. Booking assigns a driver immediately and puts the ride in `ONGOING`. There is no driver accept/reject step.
3. Pickup is given at booking; drop is given when the ride ends. Distance is the great-circle (haversine) distance between pickup and drop, rounded once to whole metres. There is no road routing.
4. Pricing tiers are marginal, like tax brackets. The spec's "first 2 km", "3-5 km", "6 km and beyond" are read as the continuous bands [0, 2 km], (2, 5 km], (5 km, infinity). A 7 km hatchback ride costs 2x10 + 3x8 + 2x5 = Rs 54.
5. Hatchback uses the spec's example tariff: minimum Rs 50, Rs 10/8/5 per km. Sedan is roughly 1.5x: minimum Rs 80, Rs 15/12/8 per km.
6. The fare is computed from the car type the user requested, never the one assigned. That is what makes the upgrade free.
7. Surge multiplier and coupon are snapshotted at booking and applied when the ride ends, so the final cost is deterministic even if the coupon is deleted or demand changes mid-ride.
8. An invalid, deleted or expired coupon fails the booking with an error; the user can rebook without it. A coupon never makes the fare negative. Percent coupons accept 1-100; flat coupons accept a positive amount in paise.
9. Default matching radius is 5 km. Candidates are available drivers of the requested car type within the radius; if none, the upgrade path is tried (Hatchback -> Sedan). An exact-type driver is preferred over an upgrade even if the upgrade is nearer.
10. New drivers start with a 5.0 rating unless one is given. Ratings never change (no rating flow in scope).
11. Cancellation is free within a grace window after booking (default 3 minutes), then a flat fee (default Rs 20). Only `ONGOING` rides can be cancelled or ended.
12. Money is integer paise; distance is integer metres; surge is an integer percent (100 = 1.00x). No floating point touches money.
13. Idempotency keys for `POST /rides` are scoped to the requesting user and have no expiry in memory. A real deployment would give them a 24h TTL.

## 3. Architecture

Layered packages with a strict inward dependency rule. `service` and `api` define the interfaces (ports) they consume; the adapters implement them by structural typing and import only `domain`. Concrete types meet only in `cmd/server`.

```
                     +------------------------------+
                     | cmd/server/main.go           |  composition root: env -> config,
                     |                              |  constructs adapters, injects into
                     +--+-----------+------------+--+  service, mounts api, serves HTTP
            constructs  |           | constructs |
  +---------------------v--+   +----v------------v-----------------------------+
  | internal/api           |-->| internal/service                              |
  | HTTP adapter           |   | use cases + PORTS it consumes:                |
  | owns port:             |   |  UserRepo DriverRepo RideRepo CouponRepo      |
  |  IdempotencyStore      |   |  FareCalculator  MatchStrategy  SurgeStrategy |
  +----------+-------------+   |  CancellationPolicy  Clock                    |
             |                 +--------------------+--------------------------+
             |                                      |
             |                 +--------------------v--------------------------+
             +---------------->| internal/domain   pure types + state machine  |<--------+
                               +-----------------------------------------------+         |
                                                                                 import domain
     +----------------+-----------------+------------------+----------------+    ONLY
+----v---------+ +----v----------+ +----v-----------+ +----v------------+  |
| store        | | pricing       | | matching       | | cancellation    |--+
| implements:  | | implements:   | | implements:    | | implements:     |
| *Repo,       | | FareCalculator| | MatchStrategy  | | CancellationPol |
| Idempotency  | | SurgeStrategy | |                | |                 |
+--------------+ +---------------+ +----------------+ +-----------------+
adapters never import service or each other
```

| Package | Responsibility |
|---|---|
| `cmd/server` | Config from env, wiring, HTTP server lifecycle, graceful shutdown. Kept thin so it is fully testable. |
| `internal/domain` | Entities, value objects (`Paise`, `Metres`, `Multiplier`, `Location`), sentinel errors, the ride state machine, ID generation. Imports nothing from `internal/`. |
| `internal/service` | Use cases: register user/driver, update location, book, end, cancel, history, coupon CRUD. Defines all repo and strategy ports. |
| `internal/api` | Routing (Go 1.22 `ServeMux` patterns), DTOs, validation, error-to-status mapping, idempotency middleware. |
| `internal/pricing` | Tariffs, marginal tiers, minimum fare, coupon discount, surge strategies. Pure functions over domain types. |
| `internal/matching` | Driver ranking strategies: `Nearest`, `HighestRated`. |
| `internal/cancellation` | Cancellation fee policies: `GracePolicy`. |
| `internal/store` | In-memory repositories, one `sync.RWMutex` per repo, atomic `Reserve`/`Release`/`Update`, idempotency store. Returns copies, never aliases. |
| `internal/web` | Browser console: one embedded HTML page served at `GET /` by the composition root. Imports nothing; uses the public API. |

## 4. Domain model

```go
type CarType string              // "HATCHBACK", "SEDAN"
func AllCarTypes() []CarType     // pricing validates a tariff exists for each
func UpgradesFor(c CarType) []CarType  // Hatchback -> [Sedan]; Sedan -> none

type Paise int64                 // money
type Metres int64                // distance
type Multiplier int              // percent; 100 = 1.00x

type Location struct{ Lat, Lng float64 }
func (l Location) DistanceTo(o Location) Metres   // haversine, mean Earth radius 6371.0088 km, rounded once

type User   struct{ ID, Name string; CreatedAt time.Time }
type Driver struct{ ID, Name string; CarType CarType; Location Location; Rating float64; Status DriverStatus }  // Available | Busy
type Coupon struct{ Code string; Kind CouponKind; Percent int; Amount Paise; ExpiresAt *time.Time }           // Kind: Percent | Flat
func (c Coupon) ValidAt(t time.Time) bool

type RideStatus string           // ONGOING | COMPLETED | CANCELLED
type Ride struct {
    ID, UserID, DriverID            string
    RequestedCarType, AssignedCarType CarType
    Upgraded                        bool
    Pickup                          Location
    Drop                            *Location
    Status                          RideStatus
    Surge                           Multiplier   // locked at booking
    Coupon                          *Coupon      // snapshot at booking
    Fare                            *Fare        // set on Complete
    CancellationFee                 Paise        // set on Cancel
    CreatedAt                       time.Time
    EndedAt                         *time.Time
}
func (r *Ride) Complete(drop Location, fare Fare, at time.Time) error   // ONGOING -> COMPLETED, else ErrInvalidTransition
func (r *Ride) Cancel(fee Paise, at time.Time) error                     // ONGOING -> CANCELLED, else ErrInvalidTransition
func (r Ride) Clone() Ride                                                // deep copy; the store never hands out aliases

type Fare struct {
    Distance        Metres
    Metered         Paise        // sum of tiers
    MinFareApplied  bool
    Base            Paise        // max(Metered, MinFare)
    Surge           Multiplier
    Surged          Paise
    Discount        Paise
    Total           Paise
}
```

Sentinel errors: `ErrNotFound`, `ErrValidation`, `ErrAlreadyExists`, `ErrNoDriverAvailable`, `ErrDriverBusy`, `ErrInvalidCoupon`, `ErrInvalidTransition`. They are wrapped with `fmt.Errorf("...: %w", err)` and matched with `errors.Is`.

IDs: `domain.NewID(prefix)` returns `prefix + "_" + 12 lowercase chars` from `crypto/rand.Text()`. Prefixes: `usr`, `drv`, `ride`. Coupons are keyed by code.

## 5. Pricing engine

```go
type Tier   struct{ UpTo Metres; RatePerKm Paise }   // UpTo == 0 means open-ended; must be last
type Tariff struct{ MinFare Paise; Tiers []Tier }

HATCHBACK: MinFare 5000,  Tiers [{2000, 1000}, {5000, 800}, {0, 500}]
SEDAN:     MinFare 8000,  Tiers [{2000, 1500}, {5000, 1200}, {0, 800}]
```

`NewCalculator(tariffs map[CarType]Tariff) (*Calculator, error)` fails fast when a car type in `AllCarTypes()` has no tariff, tiers are not strictly ascending, or the last tier is not open-ended. The same check runs as a test, so adding a car type without a tariff fails both the build and `make test`.

`Calculate(req FareRequest) (Fare, error)` with `domain.FareRequest{CarType, Distance, Surge, Coupon *Coupon}`; it rejects an unknown car type, a negative distance, a surge below 100 or an unknown coupon kind, and otherwise works in this exact order:

1. `metered = roundHalfUp(sum over tiers of metresInTier * RatePerKm, 1000)` (one rounding for the whole sum)
2. `base = max(metered, MinFare)`; `MinFareApplied = metered < MinFare`
3. `surged = roundHalfUp(base * Surge, 100)`
4. `discount = Percent: roundHalfUp(surged * Percent, 100) | Flat: Amount | nil: 0`
5. `total = max(surged - discount, 0)`

`roundHalfUp(a, b) = (a + b/2) / b` on int64. Every intermediate value is stored on `Fare` so the API can show the breakdown.

Surge strategies implement `Multiplier(in SurgeInput) domain.Multiplier` with `SurgeInput{Demand, Supply int}`. `NoSurge` returns 100. `StepSurge` holds ordered steps `{Factor int, Multiplier}` and returns the first step where `Demand <= Factor*Supply` (default steps: 1 -> 100, 2 -> 125, 3 -> 150), else the cap (200). Supply 0 with demand > 0 falls through to the cap. No integer division.

## 6. Matching and upgrade

`MatchStrategy.Rank(candidates []Driver, pickup Location) []Driver` returns candidates in preference order; the service tries to reserve them in that order. `Nearest` sorts by distance ascending. `HighestRated` sorts by rating descending, ties broken by distance.

Candidate selection is a repository query: `DriverRepo.AvailableWithin(center, radius, carType)`. The service queries the requested type first, then each type from `UpgradesFor(requested)`, stopping at the first non-empty set. `Upgraded` is true when the assigned type differs from the requested one.

## 7. Booking flow

```
BookRide(cmd{UserID, CarType, Pickup, CouponCode})
  1. validate car type; user exists                          -> ErrValidation / ErrNotFound
  2. coupon code given -> CouponRepo.Get; ValidAt(now)        -> ErrInvalidCoupon
  3. surge = Surge.Multiplier(demand, supply within radius)   computed BEFORE reserve so supply still includes this driver
  4. candidates by requested type, then upgrade path          -> ErrNoDriverAvailable if all empty
  5. ranked = Match.Rank(candidates, pickup)
  6. for d in ranked:
        ride, err := reserveAndSave(d, build)
        ErrDriverBusy -> continue (someone else won the race)
        return ride, err
  7. ErrNoDriverAvailable (every candidate was taken meanwhile)
```

```go
func (s *RideService) reserveAndSave(driverID string, build func() *domain.Ride) (*domain.Ride, error) {
    if err := s.drivers.Reserve(driverID); err != nil {
        return nil, err
    }
    committed := false
    defer func() {
        if !committed {
            s.drivers.Release(driverID)   // compensation: runs on error and on panic
        }
    }()
    ride := build()
    if err := s.rides.Save(ride); err != nil {
        return nil, err
    }
    committed = true
    return ride, nil
}
```

The defer lives in the helper, not in the ranking loop, so each iteration's compensation runs immediately.

## 8. End and cancel flows

```
EndRide(id, drop)
  ride := rides.Get(id)                                                    -> ErrNotFound
  fare := calc.Calculate(ride.RequestedCarType, ride.Pickup.DistanceTo(drop), ride.Surge, ride.Coupon)   pure
  rides.Update(id, func(r) error { return r.Complete(drop, fare, now) })   atomic under the store lock
      ErrInvalidTransition -> return; the driver is NOT touched (they may already be on another ride)
  drivers.Release(ride.DriverID); drivers.SetLocation(ride.DriverID, drop) only after the update succeeded
```

`CancelRide(id)` has the same shape: `fee := policy.Fee(ride, now)`, then `Update` with `r.Cancel(fee, now)`, then release. `RideRepo.Update` runs the mutation under the write lock, so two concurrent `End` calls on one ride produce exactly one `COMPLETED` transition and exactly one driver release.

## 9. Concurrency model

Each repository guards its map with its own `sync.RWMutex` and returns copies. No code path holds two repo locks at once. The linearization point for "two users book the same driver" is `DriverRepo.Reserve(id)`: a compare-and-set `Available -> Busy` under the write lock, returning `ErrDriverBusy` otherwise. Booking is optimistic: rank, try to reserve, fall back to the next candidate. Ride transitions are atomic through `RideRepo.Update`.

Verified by tests run under `go test -race`: 50 goroutines booking with 1 driver yield exactly 1 success; 2N goroutines with N drivers yield exactly N.

## 10. Idempotency (POST /rides)

An `api` middleware around the booking handler. Header `Idempotency-Key` is optional.

- scope = `user_id` from the request body + the key; fingerprint = SHA-256 of the body
- `IdempotencyStore.Begin(scope, fingerprint)` is atomic and returns one of: a stored response (replayed with the same status and bytes), `ErrRequestInFlight` (409 `REQUEST_IN_PROGRESS`), `ErrIdempotencyMismatch` (422 `IDEMPOTENCY_KEY_REUSED`), or nothing (proceed, marked in flight)
- the handler's status and body are captured by a recording `ResponseWriter` and stored with `Commit`; 2xx and 4xx are both stored, so a retry of a failed booking needs a new key
- a panic in the handler triggers `Abort(scope)` so the key does not stay in flight

## 11. Configuration

| Env | Default | Meaning |
|---|---|---|
| `PORT` | `8080` | listen port; `0` picks a free port (used by tests) |
| `MATCH_RADIUS_KM` | `5` | matching and surge radius |
| `MATCHING` | `nearest` | `nearest` or `rated` |
| `SURGE` | `demand` | `demand` (StepSurge) or `off` |
| `CANCEL_GRACE_MIN` | `3` | free-cancellation window |
| `CANCEL_FEE_INR` | `20` | flat fee after the window |

Numbers and ranges are checked in `loadConfig`; `MATCHING` and `SURGE` names are resolved in `newHandler`, next to the implementations, and the default tariff table travels in the config so the wiring can be tested with a broken one. Unknown values fail startup with a clear error.

## 12. API contract

All bodies are JSON. Money fields are integers in paise with a `_paise` suffix; totals also carry a human `total_inr` string.

```
GET    /users                                                                       200 {users: [...]}
GET    /drivers                                                                     200 {drivers: [...]}
GET    /coupons                                                                     200 {coupons: [...]}
POST   /users                     {name}                                            201 user
POST   /drivers                   {name, car_type, location{lat,lng}, rating?}      201 driver
PUT    /drivers/{id}/location     {lat, lng}                                        200 driver
POST   /rides                     {user_id, car_type, pickup{lat,lng}, coupon_code?} 201 ride
                                  header Idempotency-Key optional
POST   /rides/estimate            {car_type, pickup, drop, coupon_code?}            200 quote, reserves nothing
POST   /rides/{id}/end            {drop{lat,lng}}                                   200 ride with fare
POST   /rides/{id}/cancel         {}                                                200 ride with cancellation_fee_paise
GET    /users/{id}/rides?status=  ongoing|completed|cancelled, optional             200 {rides: [...]}
GET    /drivers/{id}/rides?status=                                                  200 {rides: [...]}
POST   /coupons                   {code, kind: PERCENT|FLAT, percent? | amount_paise?, expires_at?}  201 coupon
DELETE /coupons/{code}                                                              204
GET    /healthz                                                                     200 {status: ok}
GET    /                                                                            200 browser console (HTML)
```

History endpoints return rides sorted by `created_at` descending; the `status` filter is case-insensitive.

Ride JSON: `id, user_id, driver_id, requested_car_type, assigned_car_type, upgraded, status, pickup, drop, surge_multiplier (number, 1.25), coupon_code, fare, cancellation_fee_paise, created_at, ended_at`.

Fare JSON: `distance_m, metered_paise, min_fare_applied, base_paise, surge_multiplier, surged_paise, discount_paise, total_paise, total_inr`.

Error JSON: `{"error": {"code": "NO_DRIVER_AVAILABLE", "message": "..."}}`, mapped in one `statusFor(err)` function:

| Error | HTTP | code |
|---|---|---|
| `ErrValidation`: bad JSON, unknown car_type, lat/lng/rating out of range, empty name, bad coupon shape | 400 | `VALIDATION_ERROR` |
| `ErrInvalidCoupon`: unknown, deleted, expired | 400 | `INVALID_COUPON` |
| `ErrNotFound`: user, driver, ride, coupon | 404 | `NOT_FOUND` |
| `ErrAlreadyExists`: duplicate coupon code | 409 | `ALREADY_EXISTS` |
| `ErrNoDriverAvailable` | 409 | `NO_DRIVER_AVAILABLE` |
| `ErrInvalidTransition`: end or cancel a ride that is not ongoing | 409 | `INVALID_STATE` |
| idempotency in flight | 409 | `REQUEST_IN_PROGRESS` |
| idempotency fingerprint mismatch | 422 | `IDEMPOTENCY_KEY_REUSED` |
| anything else | 500 | `INTERNAL` (logged, not leaked) |

## 13. Testing and quality gates

Hard gates for every unit of work: `gofmt -l` clean, `go vet ./...`, `go test -race ./...`, 100% statement coverage via `make cover` (`-coverpkg=./...`, including `cmd/server`). `golangci-lint run` is part of `make lint` because it is installed; it is never a reason to install tooling.

| Package | What the tests prove |
|---|---|
| domain | haversine on a known pair within 1%, zero distance; `Complete`/`Cancel` from every status (table); `Coupon.ValidAt` with nil, future and past expiry; ID shape |
| pricing | `Calculate` table per car type: 0, 1 km, exactly 2 km, 2.001 km, exactly 5 km, 6 km, 7 km, 12.345 km; `MinFareApplied`; Sedan differs from Hatchback at the same distance; surge 100/125/150/200 with a half-up rounding edge; percent coupon, flat coupon, flat larger than surged gives 0; `NewCalculator` rejects a missing car type, non-ascending tiers, closed last tier; `StepSurge` table including supply 0 |
| matching | `Nearest`; `HighestRated` with a rating tie broken by distance; empty input |
| store | `Reserve` succeeds once and fails the second time; `Release`; `AvailableWithin` filters by radius, type and status; `Update` is atomic and returns the mutation's error; idempotency `Begin`/`Commit`/replay/in-flight/mismatch/`Abort` |
| service | book happy path; unknown user; no driver in radius; Hatchback -> Sedan priced as Hatchback; exact type preferred when both exist; invalid and expired coupon; coupon snapshot survives deletion; end computes the fare, releases and relocates the driver; end twice gives `ErrInvalidTransition` and leaves the driver untouched; cancel inside and after the grace window with a fake clock; history with status filter; race tests described in section 9 |
| api | `httptest`: 201 shape, 400 on a bad body, 404, every row of the status table, `Idempotency-Key` replay is byte-identical, mismatched body gives 422, in-flight gives 409 |
| cmd/server | config parsing and every invalid value; `run` starts on port 0 and stops on context cancel; `main` exits 1 on bad config via an injectable exit |

## 14. Build order

The code was built test-first in this order, one row at a time, with `go vet`, `go test -race` and the coverage gate green after every row. Rows 1-7 are the mandatory scope and were fully green before any bonus row. The author reviews the whole result and commits it.

| # | Step | Scope |
|---|---|---|
| 1 | `chore: scaffold module, make targets, lint config, spec, CLAUDE.md` | go.mod, Makefile, .golangci.yml, .gitignore, this spec, CLAUDE.md, README skeleton |
| 2 | `feat(domain): entities, money and distance types, haversine, ride state machine` | `internal/domain` + tests |
| 3 | `feat(pricing): marginal tiers, minimum fare, car-type tariffs, coupon discounts` | `internal/pricing` with `NoSurge` only + table tests |
| 4 | `feat(store): in-memory repos with atomic driver reserve and ride update` | `internal/store` (no idempotency yet) + tests |
| 5 | `feat(matching): nearest-driver strategy` | `internal/matching` + tests |
| 6 | `feat(service): registration, booking with upgrade and coupon, end ride, history` | `internal/service` incl. `reserveAndSave` and the race tests |
| 7 | `feat(api): HTTP handlers, error mapping, server wiring` | `internal/api`, `cmd/server` + tests. Mandatory scope complete. |
| 8 | `feat(api): idempotency keys for ride booking` | middleware, `store` idempotency, tests |
| 9 | `feat(pricing): demand/supply surge multiplier` | `StepSurge`, service demand/supply snapshot, `SURGE` env, tests |
| 10 | `feat(service): ride cancellation with grace-period fee policy` | `internal/cancellation`, cancel use case, endpoint, tests |
| 11 | `feat(matching): highest-rated strategy, switchable via MATCHING` | strategy + wiring + tests |
| 12 | `docs: README, demo walkthrough` | full README, `scripts/demo.sh` |

## 15. Out of scope, noted for "with more time"

Persistence (Postgres with row-level locks replacing the in-memory CAS), authentication, payments, driver accept/reject and arrival states, road-routing distance and time-based fare components, pagination, rating updates, idempotency key TTL, OpenAPI generation, structured logging and metrics.
