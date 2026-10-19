package api_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ridehailing/internal/api"
	"ridehailing/internal/cancellation"
	"ridehailing/internal/matching"
	"ridehailing/internal/pricing"
	"ridehailing/internal/service"
	"ridehailing/internal/store"
)

type rideJSON struct {
	ID                   string              `json:"id"`
	UserID               string              `json:"user_id"`
	DriverID             string              `json:"driver_id"`
	RequestedCarType     string              `json:"requested_car_type"`
	AssignedCarType      string              `json:"assigned_car_type"`
	Upgraded             bool                `json:"upgraded"`
	Status               string              `json:"status"`
	Drop                 *map[string]float64 `json:"drop"`
	SurgeMultiplier      float64             `json:"surge_multiplier"`
	CouponCode           *string             `json:"coupon_code"`
	Fare                 *fareJSON           `json:"fare"`
	CancellationFeePaise *int64              `json:"cancellation_fee_paise"`
	EndedAt              *time.Time          `json:"ended_at"`
}

type fareJSON struct {
	DistanceM       int64   `json:"distance_m"`
	MinFareApplied  bool    `json:"min_fare_applied"`
	SurgeMultiplier float64 `json:"surge_multiplier"`
	DiscountPaise   int64   `json:"discount_paise"`
	TotalPaise      int64   `json:"total_paise"`
	TotalINR        string  `json:"total_inr"`
}

type driverJSON struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	CarType  string             `json:"car_type"`
	Location map[string]float64 `json:"location"`
	Rating   float64            `json:"rating"`
	Status   string             `json:"status"`
}

type couponJSON struct {
	Code        string     `json:"code"`
	Kind        string     `json:"kind"`
	Percent     *int       `json:"percent"`
	AmountPaise *int64     `json:"amount_paise"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

type errJSON struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func newAPI(t *testing.T) (http.Handler, *store.Idempotency) {
	t.Helper()
	calc, err := pricing.NewCalculator(pricing.DefaultTariffs())
	if err != nil {
		t.Fatal(err)
	}
	users, drivers, rides, coupons := store.NewUsers(), store.NewDrivers(), store.NewRides(), store.NewCoupons()
	idem := store.NewIdempotency()
	svc := api.Services{
		Users:   service.NewUsers(users, time.Now),
		Drivers: service.NewDrivers(drivers),
		Coupons: service.NewCoupons(coupons),
		Rides: service.NewRides(service.RidesDeps{
			Users:        users,
			Drivers:      drivers,
			Rides:        rides,
			Coupons:      coupons,
			Fares:        calc,
			Match:        matching.Nearest{},
			Surge:        pricing.NoSurge{},
			Cancellation: cancellation.GracePolicy{Grace: 3 * time.Minute, Amount: 2000},
			Radius:       5000,
			Now:          time.Now,
		}),
	}
	return api.NewHandler(svc, idem, slog.New(slog.NewTextHandler(io.Discard, nil))), idem
}

func do(t *testing.T, h http.Handler, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) T {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, wantStatus, rec.Body)
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body, err)
	}
	return v
}

func assertError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	e := decode[errJSON](t, rec, status)
	if e.Error.Code != code || e.Error.Message == "" {
		t.Fatalf("error = %+v, want code %s with a message", e.Error, code)
	}
}

func createUser(t *testing.T, h http.Handler) string {
	t.Helper()
	u := decode[map[string]any](t, do(t, h, "POST", "/users", `{"name":"Asha"}`), http.StatusCreated)
	return u["id"].(string)
}

func createDriver(t *testing.T, h http.Handler, carType string, lng float64) string {
	t.Helper()
	body := fmt.Sprintf(`{"name":"Ravi","car_type":%q,"location":{"lat":0,"lng":%v}}`, carType, lng)
	return decode[driverJSON](t, do(t, h, "POST", "/drivers", body), http.StatusCreated).ID
}

func bookBody(userID string) string {
	return fmt.Sprintf(`{"user_id":%q,"car_type":"hatchback","pickup":{"lat":0,"lng":0}}`, userID)
}

func TestHealthz(t *testing.T) {
	h, _ := newAPI(t)
	if got := decode[map[string]string](t, do(t, h, "GET", "/healthz", ""), http.StatusOK); got["status"] != "ok" {
		t.Fatalf("healthz = %v", got)
	}
}

func TestCreateUser(t *testing.T) {
	h, _ := newAPI(t)
	u := decode[map[string]any](t, do(t, h, "POST", "/users", `{"name":"Asha"}`), http.StatusCreated)
	if !strings.HasPrefix(u["id"].(string), "usr_") || u["name"] != "Asha" || u["created_at"] == nil {
		t.Fatalf("user = %v", u)
	}
	for _, body := range []string{`{"name":" "}`, `{"name":`, `{"name":"A","extra":1}`, ``} {
		assertError(t, do(t, h, "POST", "/users", body), http.StatusBadRequest, "VALIDATION_ERROR")
	}
}

func TestCreateDriver(t *testing.T) {
	h, _ := newAPI(t)
	d := decode[driverJSON](t, do(t, h, "POST", "/drivers", `{"name":"Ravi","car_type":"sedan","location":{"lat":12.97,"lng":77.59}}`), http.StatusCreated)
	if d.CarType != "SEDAN" || d.Rating != 5 || d.Status != "AVAILABLE" || d.Location["lat"] != 12.97 {
		t.Fatalf("driver = %+v, want a sedan with the default 5.0 rating", d)
	}
	rated := decode[driverJSON](t, do(t, h, "POST", "/drivers", `{"name":"Ravi","car_type":"sedan","location":{"lat":0,"lng":0},"rating":4.2}`), http.StatusCreated)
	if rated.Rating != 4.2 {
		t.Fatalf("rating = %v, want 4.2", rated.Rating)
	}
	for _, body := range []string{
		`{"name":"Ravi","car_type":"sedan"}`,
		`{"name":"Ravi","car_type":"sedan","location":{"lat":0}}`,
		`{"name":"Ravi","car_type":"suv","location":{"lat":0,"lng":0}}`,
		`{"name":"Ravi","car_type":"sedan","location":{"lat":0,"lng":0},"rating":6}`,
	} {
		assertError(t, do(t, h, "POST", "/drivers", body), http.StatusBadRequest, "VALIDATION_ERROR")
	}
}

func TestUpdateDriverLocation(t *testing.T) {
	h, _ := newAPI(t)
	id := createDriver(t, h, "sedan", 0)
	d := decode[driverJSON](t, do(t, h, "PUT", "/drivers/"+id+"/location", `{"lat":1,"lng":2}`), http.StatusOK)
	if d.Location["lat"] != 1 || d.Location["lng"] != 2 {
		t.Fatalf("location = %v", d.Location)
	}
	assertError(t, do(t, h, "PUT", "/drivers/drv_missing/location", `{"lat":1,"lng":2}`), http.StatusNotFound, "NOT_FOUND")
	assertError(t, do(t, h, "PUT", "/drivers/"+id+"/location", `{"lat":1}`), http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestRideLifecycle(t *testing.T) {
	h, _ := newAPI(t)
	user := createUser(t, h)
	driver := createDriver(t, h, "hatchback", 0.01)

	ride := decode[rideJSON](t, do(t, h, "POST", "/rides", bookBody(user)), http.StatusCreated)
	if ride.Status != "ONGOING" || ride.DriverID != driver || ride.Upgraded || ride.SurgeMultiplier != 1 || ride.Fare != nil || ride.CouponCode != nil {
		t.Fatalf("ride = %+v", ride)
	}

	ended := decode[rideJSON](t, do(t, h, "POST", "/rides/"+ride.ID+"/end", `{"drop":{"lat":0,"lng":0.009}}`), http.StatusOK)
	if ended.Status != "COMPLETED" || ended.Fare == nil || ended.Drop == nil || ended.EndedAt == nil {
		t.Fatalf("ended ride = %+v", ended)
	}
	if ended.Fare.TotalPaise != 5000 || ended.Fare.TotalINR != "50.00" || !ended.Fare.MinFareApplied || ended.Fare.DistanceM != 1001 {
		t.Fatalf("fare = %+v, want the Rs 50 minimum over 1001 m", ended.Fare)
	}

	userRides := decode[map[string][]rideJSON](t, do(t, h, "GET", "/users/"+user+"/rides", ""), http.StatusOK)
	if len(userRides["rides"]) != 1 || userRides["rides"][0].ID != ride.ID {
		t.Fatalf("user rides = %+v", userRides)
	}
	completed := decode[map[string][]rideJSON](t, do(t, h, "GET", "/drivers/"+driver+"/rides?status=completed", ""), http.StatusOK)
	if len(completed["rides"]) != 1 {
		t.Fatalf("driver completed rides = %+v", completed)
	}
	ongoing := decode[map[string][]rideJSON](t, do(t, h, "GET", "/drivers/"+driver+"/rides?status=ONGOING", ""), http.StatusOK)
	if len(ongoing["rides"]) != 0 {
		t.Fatalf("driver ongoing rides = %+v", ongoing)
	}
	assertError(t, do(t, h, "GET", "/users/"+user+"/rides?status=bogus", ""), http.StatusBadRequest, "VALIDATION_ERROR")
	assertError(t, do(t, h, "GET", "/users/usr_missing/rides", ""), http.StatusNotFound, "NOT_FOUND")
	assertError(t, do(t, h, "GET", "/drivers/drv_missing/rides", ""), http.StatusNotFound, "NOT_FOUND")

	assertError(t, do(t, h, "POST", "/rides/"+ride.ID+"/end", `{"drop":{"lat":0,"lng":0}}`), http.StatusConflict, "INVALID_STATE")
	assertError(t, do(t, h, "POST", "/rides/"+ride.ID+"/cancel", ""), http.StatusConflict, "INVALID_STATE")
	assertError(t, do(t, h, "POST", "/rides/ride_missing/end", `{"drop":{"lat":0,"lng":0}}`), http.StatusNotFound, "NOT_FOUND")
	assertError(t, do(t, h, "POST", "/rides/"+ride.ID+"/end", `{}`), http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestCancelRide(t *testing.T) {
	h, _ := newAPI(t)
	user := createUser(t, h)
	createDriver(t, h, "hatchback", 0.01)
	ride := decode[rideJSON](t, do(t, h, "POST", "/rides", bookBody(user)), http.StatusCreated)

	cancelled := decode[rideJSON](t, do(t, h, "POST", "/rides/"+ride.ID+"/cancel", ""), http.StatusOK)
	if cancelled.Status != "CANCELLED" || cancelled.CancellationFeePaise == nil || *cancelled.CancellationFeePaise != 0 {
		t.Fatalf("cancelled ride = %+v, want a free cancellation inside the grace window", cancelled)
	}
	assertError(t, do(t, h, "POST", "/rides/"+ride.ID+"/cancel", ""), http.StatusConflict, "INVALID_STATE")
	assertError(t, do(t, h, "POST", "/rides/ride_missing/cancel", ""), http.StatusNotFound, "NOT_FOUND")
}

func TestBookRideErrors(t *testing.T) {
	h, _ := newAPI(t)
	user := createUser(t, h)
	assertError(t, do(t, h, "POST", "/rides", bookBody(user)), http.StatusConflict, "NO_DRIVER_AVAILABLE")
	assertError(t, do(t, h, "POST", "/rides", bookBody("usr_missing")), http.StatusNotFound, "NOT_FOUND")
	assertError(t, do(t, h, "POST", "/rides", `{"user_id":"x","car_type":"hatchback"}`), http.StatusBadRequest, "VALIDATION_ERROR")
	assertError(t, do(t, h, "POST", "/rides", `{"user_id":"x","car_type":"suv","pickup":{"lat":0,"lng":0}}`), http.StatusBadRequest, "VALIDATION_ERROR")

	createDriver(t, h, "hatchback", 0.01)
	withCoupon := fmt.Sprintf(`{"user_id":%q,"car_type":"hatchback","pickup":{"lat":0,"lng":0},"coupon_code":"NOPE"}`, user)
	assertError(t, do(t, h, "POST", "/rides", withCoupon), http.StatusBadRequest, "INVALID_COUPON")
}

func TestUpgradeIsVisible(t *testing.T) {
	h, _ := newAPI(t)
	user := createUser(t, h)
	createDriver(t, h, "sedan", 0.01)
	ride := decode[rideJSON](t, do(t, h, "POST", "/rides", bookBody(user)), http.StatusCreated)
	if ride.RequestedCarType != "HATCHBACK" || ride.AssignedCarType != "SEDAN" || !ride.Upgraded {
		t.Fatalf("ride = %+v, want a hatchback request served by a sedan", ride)
	}
	ended := decode[rideJSON](t, do(t, h, "POST", "/rides/"+ride.ID+"/end", `{"drop":{"lat":0,"lng":0.062953}}`), http.StatusOK)
	if ended.Fare.TotalPaise != 5400 {
		t.Fatalf("total = %d, want the hatchback price 5400", ended.Fare.TotalPaise)
	}
}

func TestCoupons(t *testing.T) {
	h, _ := newAPI(t)
	c := decode[couponJSON](t, do(t, h, "POST", "/coupons", `{"code":"save20","kind":"percent","percent":20}`), http.StatusCreated)
	if c.Code != "SAVE20" || c.Kind != "PERCENT" || c.Percent == nil || *c.Percent != 20 || c.AmountPaise != nil {
		t.Fatalf("coupon = %+v", c)
	}
	assertError(t, do(t, h, "POST", "/coupons", `{"code":"save20","kind":"percent","percent":20}`), http.StatusConflict, "ALREADY_EXISTS")

	flat := decode[couponJSON](t, do(t, h, "POST", "/coupons", `{"code":"flat10","kind":"flat","amount_paise":1000,"expires_at":"2030-01-01T00:00:00Z"}`), http.StatusCreated)
	if flat.Kind != "FLAT" || flat.AmountPaise == nil || *flat.AmountPaise != 1000 || flat.ExpiresAt == nil {
		t.Fatalf("coupon = %+v", flat)
	}
	for _, body := range []string{
		`{"code":"x","kind":"percent"}`,
		`{"code":"x","kind":"flat"}`,
		`{"code":"x","kind":"bogus","percent":1}`,
		`{"code":"x","kind":"percent","percent":20,"amount_paise":5}`,
		`{"code":"x","kind":"percent","percent":0}`,
	} {
		assertError(t, do(t, h, "POST", "/coupons", body), http.StatusBadRequest, "VALIDATION_ERROR")
	}

	user := createUser(t, h)
	createDriver(t, h, "hatchback", 0.01)
	body := fmt.Sprintf(`{"user_id":%q,"car_type":"hatchback","pickup":{"lat":0,"lng":0},"coupon_code":"save20"}`, user)
	ride := decode[rideJSON](t, do(t, h, "POST", "/rides", body), http.StatusCreated)
	if ride.CouponCode == nil || *ride.CouponCode != "SAVE20" {
		t.Fatalf("ride = %+v, want coupon SAVE20 attached", ride)
	}
	ended := decode[rideJSON](t, do(t, h, "POST", "/rides/"+ride.ID+"/end", `{"drop":{"lat":0,"lng":0.062953}}`), http.StatusOK)
	if ended.Fare.DiscountPaise != 1080 || ended.Fare.TotalPaise != 4320 {
		t.Fatalf("fare = %+v, want 20%% off 5400", ended.Fare)
	}

	if rec := do(t, h, "DELETE", "/coupons/save20", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
	assertError(t, do(t, h, "DELETE", "/coupons/save20", ""), http.StatusNotFound, "NOT_FOUND")
}

func TestIdempotentBooking(t *testing.T) {
	h, idem := newAPI(t)
	user := createUser(t, h)
	createDriver(t, h, "hatchback", 0.01)
	body := bookBody(user)

	first := do(t, h, "POST", "/rides", body, "Idempotency-Key", "k1")
	second := do(t, h, "POST", "/rides", body, "Idempotency-Key", "k1")
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated || first.Body.String() != second.Body.String() {
		t.Fatalf("replay differs: %d %s vs %d %s", first.Code, first.Body, second.Code, second.Body)
	}
	if second.Header().Get("Idempotent-Replayed") != "true" || first.Header().Get("Idempotent-Replayed") != "" {
		t.Fatal("only the replayed response should carry Idempotent-Replayed")
	}
	rides := decode[map[string][]rideJSON](t, do(t, h, "GET", "/users/"+user+"/rides", ""), http.StatusOK)
	if len(rides["rides"]) != 1 {
		t.Fatalf("%d rides booked, want 1", len(rides["rides"]))
	}

	other := strings.Replace(body, `"lng":0}`, `"lng":0.001}`, 1)
	assertError(t, do(t, h, "POST", "/rides", other, "Idempotency-Key", "k1"), http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED")

	sum := sha256.Sum256([]byte(body))
	if _, err := idem.Begin(user+":k2", hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	assertError(t, do(t, h, "POST", "/rides", body, "Idempotency-Key", "k2"), http.StatusConflict, "REQUEST_IN_PROGRESS")

	// the one driver is busy now, so a fresh key gets a 409 that is itself replayed
	assertError(t, do(t, h, "POST", "/rides", body, "Idempotency-Key", "k3"), http.StatusConflict, "NO_DRIVER_AVAILABLE")
	replayed := do(t, h, "POST", "/rides", body, "Idempotency-Key", "k3")
	if replayed.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatal("a stored error response should be replayed too")
	}

	huge := `{"user_id":"` + strings.Repeat("x", 2<<20) + `"}`
	assertError(t, do(t, h, "POST", "/rides", huge, "Idempotency-Key", "k4"), http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestMalformedJSONIsRejectedEverywhere(t *testing.T) {
	h, _ := newAPI(t)
	for _, ep := range []struct{ method, path string }{
		{"POST", "/users"},
		{"POST", "/drivers"},
		{"PUT", "/drivers/drv_x/location"},
		{"POST", "/rides"},
		{"POST", "/rides/ride_x/end"},
		{"POST", "/coupons"},
	} {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			assertError(t, do(t, h, ep.method, ep.path, `{"name":`), http.StatusBadRequest, "VALIDATION_ERROR")
		})
	}
	assertError(t, do(t, h, "GET", "/drivers/drv_x/rides?status=bogus", ""), http.StatusBadRequest, "VALIDATION_ERROR")
}
