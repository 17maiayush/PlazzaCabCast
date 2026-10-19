# PlazzaCabCast

A ride hailing backend in Go, built for Plazza's SDE-1 machine-coding round. It uses only the standard library and keeps everything in memory. A small browser console ships in the same binary, so you can click through the flows instead of typing curl.

Everything in the mandatory list is done, and so are all four bonus items: surge pricing, switchable matching, cancellation with a fee, and safe booking when two riders go for the same driver.

## Running it

You need Go 1.24 or newer and `make`.

```bash
make run      # API and console on http://localhost:8080
make demo     # in a second terminal: curl walkthrough of every edge case
make check    # gofmt, vet, lint, race tests and the 100% coverage gate
```

Open http://localhost:8080/ and press **Load demo data**. You get a rider, three drivers and two coupons to play with.

Other targets: `make fuzz` throws random inputs at the pricing engine, and `make e2e` clicks through the console in headless Chrome. The e2e run needs Node and a freshly started server.

## What's in it

**From the brief:** register users and drivers, book when a driver is within 5 km, end a ride and get the fare, update a cab's location, tiered pricing with a ₹50 minimum, Sedan and Hatchback rates, a free upgrade to Sedan when no Hatchback is free, coupons (add, delete, apply), and ride history for users and drivers.

**Bonus:**
- Surge goes up as demand outruns supply near the pickup, and it can be switched off with `SURGE=off`.
- `MATCHING=nearest` or `MATCHING=rated` picks the driver-matching strategy.
- Cancelling is free for 3 minutes, then costs ₹20.
- Driver reservation is a single atomic check, so one driver can never be booked twice.

**Things I added:**
- A fare estimate (`POST /rides/estimate`) that shows the price before booking.
- An `Idempotency-Key` on booking, so a retried request doesn't book a second ride.
- List endpoints, so the console reads everything from the server.

The full endpoint list and error codes are in [docs/API.md](docs/API.md).

## Assumptions

1. A driver and their cab are the same thing, so "update the cab's location" moves the driver.
2. Booking assigns a driver straight away; there is no accept or reject step. A ride goes `ONGOING`, then `COMPLETED` or `CANCELLED`.
3. Distance is the straight-line distance between pickup and drop. There is no road routing.
4. The pricing tiers stack like tax brackets: [0, 2 km] at ₹10, (2, 5 km] at ₹8, beyond 5 km at ₹5. A 7 km Hatchback ride is 20 + 24 + 10 = ₹54. A side effect is that anything under about 6.2 km lands on the ₹50 minimum.
5. The brief only prices one car type. I used it for Hatchback and set Sedan at roughly 1.5x: ₹80 minimum, then ₹15, ₹12 and ₹8 per km.
6. An invalid or expired coupon rejects the booking instead of silently booking without the discount. A discount never takes the fare below zero.
7. New drivers start with a 5.0 rating, and ratings don't change after that.

## Design in short

The code is split into layers, and dependencies only point inward. `domain` holds the core types and depends on nothing. `service` holds the use cases and declares the interfaces it needs. `api` is the HTTP layer. `store`, `pricing`, `matching` and `cancellation` are the pieces that plug in. They are wired together in exactly one file, `cmd/server/wiring.go`, so swapping a strategy is a one-line change.

The decisions that matter most:
- **The fare uses the car type the rider asked for, not the one they got.** That single rule makes the free upgrade work, with no special case anywhere.
- **Surge and the coupon are locked at booking.** The final price doesn't move mid-ride, even if the coupon is deleted.
- **Money is whole paise and distance is whole metres.** No floating point touches a fare.
- **A driver is freed only after the ride's new state is saved.** If anything fails after a driver is reserved, the reservation is rolled back.

More detail is in [ARCHITECTURE.md](ARCHITECTURE.md). The trade-offs, and what I'd replace each one with in production, are in [DECISIONS.md](DECISIONS.md).

## Testing

`make check` fails if coverage drops below 100%, and every test runs under the race detector.

The pricing tests walk the tier boundaries: 0, 1, exactly 2, 2.001, exactly 5, 6, 7 and 12.345 km. They also cover the minimum fare, both car types, surge rounding, both coupon kinds and the free upgrade.

The concurrency tests fire 50 bookings at one driver and expect exactly one to succeed.

The browser test also checks that every API route is actually used by the console.

## With more time

- Postgres instead of memory. The reservation becomes a conditional `UPDATE`, and ride changes take a row lock.
- A proper driver lifecycle: offline, accept or reject, arriving, and an ETA.
- Real road distance and trip time from a routing service.
- Ratings that update after each ride, so "highest rated" means something.
- Request IDs, metrics, pagination, and an expiry on idempotency keys.
- A geo index once there are enough drivers that scanning all of them gets slow.

## How I used AI

I built this with Claude Code in VS Code.

**How I drove it.** I gave it the assignment PDF and some context on Plazza, and asked for a design before any code. I picked Go and a REST API, then reviewed the high-level design section by section and approved each part before moving on. I set the ground rules early:
- tests first for every package
- 100% coverage, enforced by `make check`
- short comments that only explain why
- the AI never commits; I review and commit myself

**What I changed or pushed back on:**
- The first architecture diagram had the plug-in packages importing the service layer. I had it flipped, so they import only `domain` and implement interfaces the service defines.
- Ending or cancelling a ride freed the driver before checking that the ride was actually still ongoing. I made the release happen only after the state change is saved, because by then the driver might already be on someone else's ride.
- The rollback for a failed booking sat inside the candidate loop. I had it moved into its own helper, so each attempt cleans up straight away.
- Surge was worked out with integer division. I asked for a `demand <= k * supply` comparison instead, which handles zero supply without a special case.
- Idempotent booking and the fare estimate were my additions; the brief doesn't ask for either.
- The linter caught a few AI-written test helpers with parameters that never changed, and a Go 1.25 API used in a Go 1.24 module. Both were fixed.

I've read every line in this repo and can explain any of it. Where I couldn't, it came out.
