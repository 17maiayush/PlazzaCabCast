# Architecture

```
                     +------------------------------+
                     | cmd/server                   |  env -> config, builds adapters,
                     |                              |  injects them, serves HTTP
                     +--+-----------+------------+--+
            constructs  |           | constructs |
  +---------------------v--+   +----v------------v-----------------------------+
  | internal/api           |-->| internal/service                              |
  | HTTP adapter           |   | use cases + the PORTS they consume:           |
  | owns port:             |   |  UserRepo DriverRepo RideRepo CouponRepo      |
  |  IdempotencyStore      |   |  FareCalculator MatchStrategy SurgeStrategy   |
  +----------+-------------+   |  CancellationPolicy Clock                     |
             |                 +--------------------+--------------------------+
             |                                      |
             |                 +--------------------v--------------------------+
             +---------------->| internal/domain   pure types + state machine  |<--------+
                               +-----------------------------------------------+         |
                                                                                 import domain
     +----------------+-----------------+------------------+----------------+    ONLY
+----v---------+ +----v----------+ +----v-----------+ +----v------------+  |
| store        | | pricing       | | matching       | | cancellation    |--+
| in-memory    | | tiers, surge  | | Nearest,       | | GracePolicy     |
| repos        | | coupons       | | HighestRated   | |                 |
+--------------+ +---------------+ +----------------+ +-----------------+
```

**Dependency rule.** Arrows only point inward. `domain` imports nothing from the project. `service` and `api` declare the interfaces they need (the Go idiom: the consumer owns the interface). The adapters import only `domain` and satisfy those interfaces structurally; they never import `service` or each other. Concrete types meet in exactly one file, `cmd/server/wiring.go`. The console in `internal/web` imports nothing; it talks to the API over HTTP like any other client.

**Where each strategy lives, and how to switch it.**

| Concern | Interface (in `service`) | Implementations | Switch |
|---|---|---|---|
| Fare | `FareCalculator` | `pricing.Calculator` over a `Tariff` per car type | edit `pricing.DefaultTariffs` |
| Surge | `SurgeStrategy` | `pricing.NoSurge`, `pricing.StepSurge` | `SURGE=off\|demand` |
| Driver choice | `MatchStrategy` | `matching.Nearest`, `matching.HighestRated` | `MATCHING=nearest\|rated` |
| Cancellation fee | `CancellationPolicy` | `cancellation.GracePolicy` | `CANCEL_GRACE_MIN`, `CANCEL_FEE_INR` |
| Storage | `UserRepo`, `DriverRepo`, `RideRepo`, `CouponRepo` | `store.*` (in-memory) | wiring |

The booking code in `service/rides.go` calls `Rank`, `Multiplier`, `Calculate` and `Fee` through the interfaces and never names an implementation. Swapping one is a one-line change in the wiring.

**Booking, step by step** (`service.Rides.Book`):

1. Validate the car type and pickup; the user must exist; a coupon, if given, must exist and not be expired.
2. Read the surge multiplier from a demand/supply snapshot around the pickup. This happens *before* reserving, so the supply count still includes the driver about to be taken.
3. Query available drivers of the requested type within the radius. If none, walk the upgrade path (Hatchback -> Sedan). If still none, `NO_DRIVER_AVAILABLE`.
4. Rank the candidates with the matching strategy.
5. For each candidate in order: `Reserve` the driver (an atomic compare-and-set in the store), build the ride with the surge and coupon snapshot, save it. If the reserve fails because another booking won the race, move to the next candidate. Any failure after a successful reserve, including a panic, releases the driver again (a deferred compensation in `reserveAndSave`).

**Ending and cancelling.** The fare is computed first because it is pure. The state transition then runs inside `RideRepo.Update`, under the store's write lock, so two concurrent `end` calls on one ride produce exactly one `COMPLETED`. Only after the transition is saved is the driver released and moved to the drop point. On `INVALID_STATE` the driver is never touched: they may already be serving someone else.

**Concurrency model.** Every repository guards its own map with a `sync.RWMutex` and hands out copies, never references. No code path holds two repository locks at once. The one linearization point for "two riders, one driver" is `store.Drivers.Reserve`. Bookings are optimistic and retry down the ranked list. The tests spawn 50 goroutines against one driver and 20 against ten, under `-race`, and assert exactly one and exactly ten successes.

**Idempotency.** `POST /rides` with an `Idempotency-Key` is wrapped by a middleware in `api`. The claim is scoped to `user_id + key` and carries a SHA-256 of the body. Repeating the request replays the stored status and bytes with an `Idempotent-Replayed: true` header; reusing the key with a different body is a 422; a concurrent duplicate while the first is running is a 409. Both 2xx and 4xx results are stored, so a client that wants a fresh attempt sends a fresh key. A panic in the handler releases the claim.

**Money.** `domain.Paise` and `domain.Metres` are int64. The fare pipeline is pure integer arithmetic: sum paise-metres across tiers and round once, apply the minimum, multiply by the surge percent and round, subtract the discount and round, floor at zero. Every intermediate value is kept on the `Fare` so the API can show its work.


## Extending

Each change below is local to one or two files, and a test tells you when you are done.

- **Add a car type (e.g. SUV).** Add the constant and list it in `domain.AllCarTypes`. `TestDefaultTariffsCoverEveryCarType` and `NewCalculator` now fail until you add a tariff in `pricing.DefaultTariffs`. Optionally add it to the upgrade map in `domain/types.go`. The API accepts `"suv"` with no further change.
- **Add or change a pricing tier.** It is data in `pricing.DefaultTariffs`; add a row to `TestHatchbackTiersAndMinimumFare`.
- **Add a coupon kind.** New `domain.CouponKind` constant and constructor, one `case` in `pricing.discountFor` (the `exhaustive` linter flags it if you forget), one `case` in `api.createCouponRequest.toDomain`.
- **Add a matching strategy.** A type with `Rank` in `matching`, one `case` in `cmd/server.matchStrategy`.
- **Add a surge or cancellation policy.** A type with `Multiplier` in `pricing`, or `Fee` in `cancellation`, plus a `case` in the wiring.

