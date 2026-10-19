package store_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ridehailing/internal/domain"
	"ridehailing/internal/store"
)

var (
	now    = time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)
	origin = domain.Location{}
)

func driver(t *testing.T, carType domain.CarType, loc domain.Location) domain.Driver {
	t.Helper()
	d, err := domain.NewDriver("d", carType, loc, 4)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestUsers(t *testing.T) {
	users := store.NewUsers()
	u, _ := domain.NewUser("Asha", now)
	if err := users.Save(u); err != nil {
		t.Fatal(err)
	}
	if err := users.Save(u); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate save error = %v, want ErrAlreadyExists", err)
	}
	got, err := users.Get(u.ID)
	if err != nil || got != u {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, u)
	}
	if _, err := users.Get("usr_missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing error = %v, want ErrNotFound", err)
	}
}

func TestDriversSaveGetAndLocation(t *testing.T) {
	drivers := store.NewDrivers()
	d := driver(t, domain.Sedan, origin)
	if err := drivers.Save(d); err != nil {
		t.Fatal(err)
	}
	if err := drivers.Save(d); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate save error = %v, want ErrAlreadyExists", err)
	}
	moved := domain.Location{Lat: 1, Lng: 1}
	if err := drivers.SetLocation(d.ID, moved); err != nil {
		t.Fatal(err)
	}
	got, err := drivers.Get(d.ID)
	if err != nil || got.Location != moved {
		t.Fatalf("Get after move = %+v, %v", got, err)
	}
	if err := drivers.SetLocation("drv_missing", moved); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetLocation missing error = %v, want ErrNotFound", err)
	}
	if _, err := drivers.Get("drv_missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get missing error = %v, want ErrNotFound", err)
	}
}

func TestDriversAvailableWithin(t *testing.T) {
	drivers := store.NewDrivers()
	near := driver(t, domain.Hatchback, domain.Location{Lng: 0.01}) // ~1.1 km
	far := driver(t, domain.Hatchback, domain.Location{Lng: 0.1})   // ~11 km
	sedan := driver(t, domain.Sedan, domain.Location{Lng: 0.02})
	busy := driver(t, domain.Hatchback, domain.Location{Lng: 0.03})
	edge := driver(t, domain.Hatchback, domain.Location{Lng: 1}) // exactly 111195 m
	for _, d := range []domain.Driver{near, far, sedan, busy, edge} {
		if err := drivers.Save(d); err != nil {
			t.Fatal(err)
		}
	}
	if err := drivers.Reserve(busy.ID); err != nil {
		t.Fatal(err)
	}

	got := drivers.AvailableWithin(origin, 5000, domain.Hatchback)
	if len(got) != 1 || got[0].ID != near.ID {
		t.Fatalf("AvailableWithin 5 km hatchback = %+v, want only the near driver", got)
	}
	if got := drivers.AvailableWithin(origin, 111195, domain.Hatchback); len(got) != 3 {
		t.Fatalf("radius boundary should be inclusive, got %d drivers", len(got))
	}
	if got := drivers.CountAvailableWithin(origin, 5000); got != 2 {
		t.Fatalf("CountAvailableWithin = %d, want 2 (near hatchback + sedan)", got)
	}
}

func TestDriversReserveAndRelease(t *testing.T) {
	drivers := store.NewDrivers()
	d := driver(t, domain.Sedan, origin)
	if err := drivers.Save(d); err != nil {
		t.Fatal(err)
	}
	if err := drivers.Reserve(d.ID); err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if err := drivers.Reserve(d.ID); !errors.Is(err, domain.ErrDriverBusy) {
		t.Fatalf("second reserve error = %v, want ErrDriverBusy", err)
	}
	if got, _ := drivers.Get(d.ID); got.Status != domain.DriverBusy {
		t.Fatalf("status after reserve = %s, want BUSY", got.Status)
	}
	if err := drivers.Release(d.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := drivers.Get(d.ID); got.Status != domain.DriverAvailable {
		t.Fatalf("status after release = %s, want AVAILABLE", got.Status)
	}
	if err := drivers.Release(d.ID); err != nil {
		t.Fatalf("releasing an available driver should be a no-op, got %v", err)
	}
	if err := drivers.Reserve("drv_missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("reserve missing error = %v, want ErrNotFound", err)
	}
	if err := drivers.Release("drv_missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("release missing error = %v, want ErrNotFound", err)
	}
}

func TestDriversReserveIsAtomic(t *testing.T) {
	drivers := store.NewDrivers()
	d := driver(t, domain.Sedan, origin)
	if err := drivers.Save(d); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var wins atomic.Int32
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if drivers.Reserve(d.ID) == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("%d goroutines reserved the same driver, want exactly 1", wins.Load())
	}
}

func ride(t *testing.T, userID string, d domain.Driver, pickup domain.Location, at time.Time) domain.Ride {
	t.Helper()
	return domain.NewRide(userID, d, d.CarType, pickup, 100, nil, at)
}

func TestRidesSaveGetUpdate(t *testing.T) {
	rides := store.NewRides()
	d := driver(t, domain.Sedan, origin)
	r := ride(t, "usr_1", d, origin, now)
	if err := rides.Save(r); err != nil {
		t.Fatal(err)
	}
	if err := rides.Save(r); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate save error = %v, want ErrAlreadyExists", err)
	}
	if _, err := rides.Get("ride_missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing error = %v, want ErrNotFound", err)
	}

	updated, err := rides.Update(r.ID, func(r *domain.Ride) error {
		return r.Complete(domain.Location{Lat: 1}, domain.Fare{Total: 5400}, now.Add(time.Minute))
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != domain.RideCompleted || updated.Fare.Total != 5400 {
		t.Fatalf("Update returned %+v", updated)
	}
	updated.Fare.Total = 1
	if stored, _ := rides.Get(r.ID); stored.Fare.Total != 5400 {
		t.Fatal("Update returned a ride that aliases the stored one")
	}

	_, err = rides.Update(r.ID, func(r *domain.Ride) error { return r.Cancel(0, now) })
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("Update error = %v, want the mutation's ErrInvalidTransition", err)
	}
	if stored, _ := rides.Get(r.ID); stored.Status != domain.RideCompleted {
		t.Fatal("a failed mutation must not change the stored ride")
	}
	if _, err := rides.Update("ride_missing", func(*domain.Ride) error { return nil }); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update missing error = %v, want ErrNotFound", err)
	}
}

func TestRidesGetReturnsACopy(t *testing.T) {
	rides := store.NewRides()
	coupon, _ := domain.NewPercentCoupon("C", 10, nil)
	r := domain.NewRide("usr_1", driver(t, domain.Sedan, origin), domain.Sedan, origin, 100, &coupon, now)
	if err := rides.Save(r); err != nil {
		t.Fatal(err)
	}
	got, _ := rides.Get(r.ID)
	got.Coupon.Percent = 99
	if again, _ := rides.Get(r.ID); again.Coupon.Percent != 10 {
		t.Fatal("Get returned a ride that aliases the stored one")
	}
}

func TestRidesListAndCount(t *testing.T) {
	rides := store.NewRides()
	d1 := driver(t, domain.Sedan, origin)
	d2 := driver(t, domain.Sedan, origin)
	oldest := ride(t, "usr_1", d1, origin, now)
	middle := ride(t, "usr_1", d2, origin, now.Add(time.Minute))
	newest := ride(t, "usr_1", d1, domain.Location{Lng: 0.1}, now.Add(2*time.Minute)) // ~11 km away
	other := ride(t, "usr_2", d2, origin, now.Add(3*time.Minute))
	for _, r := range []domain.Ride{oldest, middle, newest, other} {
		if err := rides.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := rides.Update(middle.ID, func(r *domain.Ride) error { return r.Complete(origin, domain.Fare{}, now) }); err != nil {
		t.Fatal(err)
	}

	got := rides.ListByUser("usr_1", nil)
	if len(got) != 3 || got[0].ID != newest.ID || got[1].ID != middle.ID || got[2].ID != oldest.ID {
		t.Fatalf("ListByUser order = %v", ids(got))
	}
	completed := domain.RideCompleted
	if got := rides.ListByUser("usr_1", &completed); len(got) != 1 || got[0].ID != middle.ID {
		t.Fatalf("ListByUser completed = %v", ids(got))
	}
	if got := rides.ListByDriver(d1.ID, nil); len(got) != 2 || got[0].ID != newest.ID || got[1].ID != oldest.ID {
		t.Fatalf("ListByDriver = %v", ids(got))
	}
	if got := rides.ListByUser("usr_nobody", nil); len(got) != 0 {
		t.Fatalf("unknown user should have no rides, got %v", ids(got))
	}
	if got := rides.CountOngoingWithin(origin, 5000); got != 2 {
		t.Fatalf("CountOngoingWithin = %d, want 2 (oldest + other; middle completed, newest far)", got)
	}
}

func ids(rides []domain.Ride) []string {
	out := make([]string, len(rides))
	for i, r := range rides {
		out[i] = r.ID
	}
	return out
}

func TestCoupons(t *testing.T) {
	coupons := store.NewCoupons()
	c, _ := domain.NewPercentCoupon("SAVE20", 20, nil)
	if err := coupons.Save(c); err != nil {
		t.Fatal(err)
	}
	if err := coupons.Save(c); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate save error = %v, want ErrAlreadyExists", err)
	}
	got, err := coupons.Get("SAVE20")
	if err != nil || got.Percent != 20 {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if err := coupons.Delete("SAVE20"); err != nil {
		t.Fatal(err)
	}
	if _, err := coupons.Get("SAVE20"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get after delete error = %v, want ErrNotFound", err)
	}
	if err := coupons.Delete("SAVE20"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second delete error = %v, want ErrNotFound", err)
	}
}

func TestIdempotency(t *testing.T) {
	idem := store.NewIdempotency()
	if replay, err := idem.Begin("u1:k1", "fp"); replay != nil || err != nil {
		t.Fatalf("first Begin = %v, %v; want nil, nil", replay, err)
	}
	if _, err := idem.Begin("u1:k1", "fp"); !errors.Is(err, domain.ErrRequestInFlight) {
		t.Fatalf("Begin while in flight error = %v, want ErrRequestInFlight", err)
	}
	if _, err := idem.Begin("u1:k1", "other"); !errors.Is(err, domain.ErrIdempotencyMismatch) {
		t.Fatalf("Begin with another body while in flight error = %v, want ErrIdempotencyMismatch", err)
	}

	idem.Commit("u1:k1", domain.IdempotentResponse{Status: 201, Body: []byte(`{"id":"ride_1"}`)})
	replay, err := idem.Begin("u1:k1", "fp")
	if err != nil || replay == nil || replay.Status != 201 || string(replay.Body) != `{"id":"ride_1"}` {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	replay.Body[0] = 'X'
	if again, _ := idem.Begin("u1:k1", "fp"); string(again.Body) != `{"id":"ride_1"}` {
		t.Fatal("replayed body aliases the stored one")
	}
	if _, err := idem.Begin("u1:k1", "other"); !errors.Is(err, domain.ErrIdempotencyMismatch) {
		t.Fatalf("Begin with another body after commit error = %v, want ErrIdempotencyMismatch", err)
	}

	if _, err := idem.Begin("u1:k2", "fp"); err != nil {
		t.Fatal(err)
	}
	idem.Abort("u1:k2")
	if replay, err := idem.Begin("u1:k2", "fp"); replay != nil || err != nil {
		t.Fatalf("Begin after Abort = %v, %v; want nil, nil", replay, err)
	}

	idem.Abort("u1:k3")
	idem.Commit("u1:k3", domain.IdempotentResponse{Status: 201})
	if replay, err := idem.Begin("u1:k3", "fp"); replay != nil || err != nil {
		t.Fatalf("Commit without a claim should be a no-op, Begin = %v, %v", replay, err)
	}
	idem.Abort("u1:k1")
	if replay, _ := idem.Begin("u1:k1", "fp"); replay == nil {
		t.Fatal("Abort must not drop a committed response")
	}
}
