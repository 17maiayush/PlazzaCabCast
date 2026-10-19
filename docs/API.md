# API reference

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

Errors come back as `{"error": {"code": "...", "message": "..."}}`:

| HTTP | code | when |
|---|---|---|
| 400 | `VALIDATION_ERROR` | bad JSON, unknown field, missing coordinate, unknown car type, rating or coupon value out of range |
| 400 | `INVALID_COUPON` | unknown, deleted or expired coupon at booking |
| 404 | `NOT_FOUND` | user, driver, ride or coupon does not exist |
| 409 | `ALREADY_EXISTS` | duplicate coupon code |
| 409 | `NO_DRIVER_AVAILABLE` | nothing bookable within the radius, after trying the upgrade path |
| 409 | `INVALID_STATE` | ending or cancelling a ride that is not ongoing |
| 409 | `REQUEST_IN_PROGRESS` | same Idempotency-Key, first request still running |
| 422 | `IDEMPOTENCY_KEY_REUSED` | same Idempotency-Key with a different body |
| 500 | `INTERNAL` | anything else; the cause is logged, never returned |

A completed ride looks like this:

```json
{
  "id": "ride_fbedzpdgmhmy", "user_id": "usr_rda3u2a3salv", "driver_id": "drv_7hk7opw5nnc7",
  "requested_car_type": "HATCHBACK", "assigned_car_type": "SEDAN", "upgraded": true,
  "status": "COMPLETED", "surge_multiplier": 1, "coupon_code": null,
  "fare": {
    "distance_m": 15575, "metered_paise": 9688, "min_fare_applied": false, "base_paise": 9688,
    "surge_multiplier": 1, "surged_paise": 9688, "discount_paise": 0, "total_paise": 9688, "total_inr": "96.88"
  },
  "cancellation_fee_paise": null, "created_at": "...", "ended_at": "..."
}
```

That ride asked for a Hatchback, got a Sedan, and paid the Hatchback price. The Sedan tariff would have been Rs 150.60.


## Estimate

`POST /rides/estimate` runs the same validation, coupon check, surge snapshot, upgrade search and pricing as a booking, then stops before reserving anyone. The price can still change before `POST /rides`, because surge and availability are live.

```json
{
  "requested_car_type": "HATCHBACK", "assigned_car_type": "SEDAN", "driver_available": true, "upgraded": true,
  "coupon_code": "SAVE20",
  "fare": { "distance_m": 7000, "metered_paise": 5400, "min_fare_applied": false, "base_paise": 5400,
            "surge_multiplier": 1, "surged_paise": 5400, "discount_paise": 1080, "total_paise": 4320, "total_inr": "43.20" }
}
```

No driver in range is not an error here: `driver_available` is false and `assigned_car_type` is null, so a client can still show the price.
