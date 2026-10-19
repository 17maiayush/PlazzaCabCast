package service_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"ridehailing/internal/cancellation"
	"ridehailing/internal/domain"
	"ridehailing/internal/matching"
	"ridehailing/internal/pricing"
	"ridehailing/internal/service"
	"ridehailing/internal/store"
)

var (
	start  = time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)
	pickup = domain.Location{}
	// 7000 m east of the origin; the hatchback fare for it is Rs 54, the sedan fare Rs 82.
	sevenKm = domain.Location{Lng: 0.062953}
	oneKm   = domain.Location{Lng: 0.009}
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type harness struct {
	drivers   *store.Drivers
	clock     *fakeClock
	users     *service.Users
	driverOps *service.Drivers
	coupons   *service.Coupons
	rides     *service.Rides
}

func newHarness(t *testing.T, tweak ...func(*service.RidesDeps)) *harness {
	t.Helper()
	calc, err := pricing.NewCalculator(pricing.DefaultTariffs())
	if err != nil {
		t.Fatal(err)
	}
	users, drivers, rides, coupons := store.NewUsers(), store.NewDrivers(), store.NewRides(), store.NewCoupons()
	clock := &fakeClock{t: start}
	deps := service.RidesDeps{
		Users:        users,
		Drivers:      drivers,
		Rides:        rides,
		Coupons:      coupons,
		Fares:        calc,
		Match:        matching.Nearest{},
		Surge:        pricing.NoSurge{},
		Cancellation: cancellation.GracePolicy{Grace: 3 * time.Minute, Amount: 2000},
		Radius:       5000,
		Now:          clock.Now,
	}
	for _, fn := range tweak {
		fn(&deps)
	}
	return &harness{
		drivers:   drivers,
		clock:     clock,
		users:     service.NewUsers(users, clock.Now),
		driverOps: service.NewDrivers(drivers),
		coupons:   service.NewCoupons(coupons),
		rides:     service.NewRides(deps),
	}
}

func (h *harness) user(t *testing.T) domain.User {
	t.Helper()
	u, err := h.users.Register("Asha")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (h *harness) driver(t *testing.T, carType domain.CarType, lng, rating float64) domain.Driver {
	t.Helper()
	d, err := h.driverOps.Register("Ravi", carType, domain.Location{Lng: lng}, rating)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (h *harness) book(t *testing.T, userID, coupon string) domain.Ride {
	t.Helper()
	r, err := h.rides.Book(service.BookRequest{UserID: userID, CarType: domain.Hatchback, Pickup: pickup, CouponCode: coupon})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	return r
}

func (h *harness) driverStatus(t *testing.T, id string) domain.DriverStatus {
	t.Helper()
	d, err := h.drivers.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	return d.Status
}

func TestUsersRegister(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	if u.ID == "" || u.Name != "Asha" || !u.CreatedAt.Equal(start) {
		t.Fatalf("unexpected user %+v", u)
	}
	if _, err := h.users.Register(" "); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
}

func TestDriversRegisterAndUpdateLocation(t *testing.T) {
	h := newHarness(t)
	d := h.driver(t, domain.Sedan, 0.01, 4.5)
	if d.Status != domain.DriverAvailable {
		t.Fatalf("new driver status = %s, want AVAILABLE", d.Status)
	}
	if _, err := h.driverOps.Register("Ravi", "SUV", pickup, 4); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}

	moved, err := h.driverOps.UpdateLocation(d.ID, sevenKm)
	if err != nil || moved.Location != sevenKm {
		t.Fatalf("UpdateLocation = %+v, %v", moved, err)
	}
	if _, err := h.driverOps.UpdateLocation(d.ID, domain.Location{Lat: 91}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
	if _, err := h.driverOps.UpdateLocation("drv_missing", pickup); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestCouponsAddAndDelete(t *testing.T) {
	h := newHarness(t)
	c, _ := domain.NewPercentCoupon("SAVE20", 20, nil)
	if err := h.coupons.Add(c); err != nil {
		t.Fatal(err)
	}
	if err := h.coupons.Add(c); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("error = %v, want ErrAlreadyExists", err)
	}
	if err := h.coupons.Delete(" save20 "); err != nil {
		t.Fatalf("Delete should normalise the code: %v", err)
	}
	if err := h.coupons.Delete("SAVE20"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestBookPrefersRequestedTypeOverNearerUpgrade(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	h.driver(t, domain.Hatchback, 0.02, 4)
	near := h.driver(t, domain.Hatchback, 0.01, 4)
	h.driver(t, domain.Sedan, 0.005, 5)

	r := h.book(t, u.ID, "")
	if r.DriverID != near.ID || r.Upgraded || r.AssignedCarType != domain.Hatchback {
		t.Fatalf("unexpected ride %+v", r)
	}
	if r.Status != domain.RideOngoing || r.Surge != 100 || r.Coupon != nil || !r.CreatedAt.Equal(start) {
		t.Fatalf("unexpected ride %+v", r)
	}
	if h.driverStatus(t, near.ID) != domain.DriverBusy {
		t.Fatal("assigned driver should be BUSY")
	}
}

func TestBookUpgradesToSedanAtHatchbackPrice(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	sedan := h.driver(t, domain.Sedan, 0.01, 4)

	r := h.book(t, u.ID, "")
	if r.DriverID != sedan.ID || !r.Upgraded || r.AssignedCarType != domain.Sedan || r.RequestedCarType != domain.Hatchback {
		t.Fatalf("unexpected ride %+v", r)
	}
	ended, err := h.rides.End(r.ID, sevenKm)
	if err != nil {
		t.Fatal(err)
	}
	if ended.Fare.Distance != 7000 || ended.Fare.Total != 5400 {
		t.Fatalf("fare = %+v, want the hatchback price of 5400 over 7000 m", ended.Fare)
	}
}

func TestBookRejections(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	far := h.driver(t, domain.Hatchback, 0.1, 4)
	busy := h.driver(t, domain.Hatchback, 0.01, 4)
	if err := h.drivers.Reserve(busy.ID); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		req  service.BookRequest
		want error
	}{
		{"unknown user", service.BookRequest{UserID: "usr_x", CarType: domain.Hatchback, Pickup: pickup}, domain.ErrNotFound},
		{"unknown car type", service.BookRequest{UserID: u.ID, CarType: "SUV", Pickup: pickup}, domain.ErrValidation},
		{"bad pickup", service.BookRequest{UserID: u.ID, CarType: domain.Hatchback, Pickup: domain.Location{Lat: 91}}, domain.ErrValidation},
		{"no driver in radius", service.BookRequest{UserID: u.ID, CarType: domain.Hatchback, Pickup: pickup}, domain.ErrNoDriverAvailable},
		{"unknown coupon", service.BookRequest{UserID: u.ID, CarType: domain.Hatchback, Pickup: pickup, CouponCode: "NOPE"}, domain.ErrInvalidCoupon},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.rides.Book(tc.req); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
	if h.driverStatus(t, far.ID) != domain.DriverAvailable {
		t.Fatal("a rejected booking must not leave a driver reserved")
	}
}

func TestBookCouponIsSnapshotAtBooking(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	h.driver(t, domain.Hatchback, 0.01, 4)
	c, _ := domain.NewPercentCoupon("SAVE20", 20, nil)
	if err := h.coupons.Add(c); err != nil {
		t.Fatal(err)
	}

	r := h.book(t, u.ID, " save20 ")
	if r.Coupon == nil || r.Coupon.Code != "SAVE20" {
		t.Fatalf("coupon not attached: %+v", r)
	}
	if err := h.coupons.Delete("SAVE20"); err != nil {
		t.Fatal(err)
	}
	ended, err := h.rides.End(r.ID, sevenKm)
	if err != nil {
		t.Fatal(err)
	}
	if ended.Fare.Discount != 1080 || ended.Fare.Total != 4320 {
		t.Fatalf("fare = %+v, want the 20%% discount even though the coupon was deleted", ended.Fare)
	}
}

func TestBookRejectsExpiredCoupon(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	d := h.driver(t, domain.Hatchback, 0.01, 4)
	exp := start
	c, _ := domain.NewPercentCoupon("OLD", 20, &exp)
	if err := h.coupons.Add(c); err != nil {
		t.Fatal(err)
	}
	if _, err := h.rides.Book(service.BookRequest{UserID: u.ID, CarType: domain.Hatchback, Pickup: pickup, CouponCode: "OLD"}); !errors.Is(err, domain.ErrInvalidCoupon) {
		t.Fatalf("error = %v, want ErrInvalidCoupon", err)
	}
	if h.driverStatus(t, d.ID) != domain.DriverAvailable {
		t.Fatal("coupon must be checked before any driver is reserved")
	}
}

func TestEndRideReleasesAndMovesDriver(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	d := h.driver(t, domain.Hatchback, 0.01, 4)
	r := h.book(t, u.ID, "")
	h.clock.Advance(20 * time.Minute)

	ended, err := h.rides.End(r.ID, oneKm)
	if err != nil {
		t.Fatal(err)
	}
	if ended.Status != domain.RideCompleted || !ended.EndedAt.Equal(start.Add(20*time.Minute)) || *ended.Drop != oneKm {
		t.Fatalf("unexpected ride %+v", ended)
	}
	if !ended.Fare.MinFareApplied || ended.Fare.Total != 5000 {
		t.Fatalf("fare = %+v, want the Rs 50 minimum", ended.Fare)
	}
	driver, _ := h.drivers.Get(d.ID)
	if driver.Status != domain.DriverAvailable || driver.Location != oneKm {
		t.Fatalf("driver after end = %+v, want AVAILABLE at the drop", driver)
	}

	if _, err := h.rides.End(r.ID, domain.Location{Lat: 91}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("error = %v, want ErrValidation", err)
	}
	if _, err := h.rides.End("ride_missing", oneKm); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestEndTwiceLeavesTheDriverAlone(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	d := h.driver(t, domain.Hatchback, 0.01, 4)
	first := h.book(t, u.ID, "")
	if _, err := h.rides.End(first.ID, oneKm); err != nil {
		t.Fatal(err)
	}
	second := h.book(t, u.ID, "")
	if second.DriverID != d.ID {
		t.Fatal("expected the same driver to be reused")
	}

	if _, err := h.rides.End(first.ID, oneKm); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("error = %v, want ErrInvalidTransition", err)
	}
	if h.driverStatus(t, d.ID) != domain.DriverBusy {
		t.Fatal("ending an already-ended ride must not release the driver's new ride")
	}
}

func TestCancel(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	d := h.driver(t, domain.Hatchback, 0.01, 4)

	free := h.book(t, u.ID, "")
	h.clock.Advance(3 * time.Minute)
	cancelled, err := h.rides.Cancel(free.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != domain.RideCancelled || cancelled.CancellationFee != 0 || !cancelled.EndedAt.Equal(h.clock.Now()) {
		t.Fatalf("unexpected ride %+v", cancelled)
	}
	if h.driverStatus(t, d.ID) != domain.DriverAvailable {
		t.Fatal("cancel must release the driver")
	}

	late := h.book(t, u.ID, "")
	h.clock.Advance(3*time.Minute + time.Second)
	if cancelled, err := h.rides.Cancel(late.ID); err != nil || cancelled.CancellationFee != 2000 {
		t.Fatalf("late cancel = %+v, %v; want a Rs 20 fee", cancelled, err)
	}
	if _, err := h.rides.Cancel(late.ID); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("error = %v, want ErrInvalidTransition", err)
	}

	done := h.book(t, u.ID, "")
	if _, err := h.rides.End(done.ID, oneKm); err != nil {
		t.Fatal(err)
	}
	if _, err := h.rides.Cancel(done.ID); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("error = %v, want ErrInvalidTransition", err)
	}
	if _, err := h.rides.Cancel("ride_missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestHistory(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	d := h.driver(t, domain.Hatchback, 0.01, 4)
	first := h.book(t, u.ID, "")
	if _, err := h.rides.End(first.ID, oneKm); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Minute)
	second := h.book(t, u.ID, "")

	all, err := h.rides.ForUser(u.ID, nil)
	if err != nil || len(all) != 2 || all[0].ID != second.ID || all[1].ID != first.ID {
		t.Fatalf("ForUser = %+v, %v", all, err)
	}
	ongoing := domain.RideOngoing
	if got, _ := h.rides.ForUser(u.ID, &ongoing); len(got) != 1 || got[0].ID != second.ID {
		t.Fatalf("ForUser ongoing = %+v", got)
	}
	completed := domain.RideCompleted
	if got, _ := h.rides.ForDriver(d.ID, &completed); len(got) != 1 || got[0].ID != first.ID {
		t.Fatalf("ForDriver completed = %+v", got)
	}
	if _, err := h.rides.ForUser("usr_missing", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if _, err := h.rides.ForDriver("drv_missing", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestSurgeIsLockedAtBooking(t *testing.T) {
	h := newHarness(t, func(d *service.RidesDeps) { d.Surge = pricing.DefaultStepSurge() })
	u := h.user(t)
	for range 3 {
		h.driver(t, domain.Hatchback, 0.01, 4)
	}

	if r := h.book(t, u.ID, ""); r.Surge != 100 {
		t.Fatalf("first booking surge = %d, want 100 (demand 0, supply 3)", r.Surge)
	}
	h.book(t, u.ID, "")
	third := h.book(t, u.ID, "")
	if third.Surge != 125 {
		t.Fatalf("third booking surge = %d, want 125 (demand 2, supply 1)", third.Surge)
	}
	ended, err := h.rides.End(third.ID, sevenKm)
	if err != nil {
		t.Fatal(err)
	}
	if ended.Fare.Surge != 125 || ended.Fare.Surged != 6750 || ended.Fare.Total != 6750 {
		t.Fatalf("fare = %+v, want the 1.25x locked at booking", ended.Fare)
	}
}

func TestHighestRatedStrategyIsSwappable(t *testing.T) {
	h := newHarness(t, func(d *service.RidesDeps) { d.Match = matching.HighestRated{} })
	u := h.user(t)
	h.driver(t, domain.Hatchback, 0.01, 3.9)
	best := h.driver(t, domain.Hatchback, 0.03, 4.9)
	if r := h.book(t, u.ID, ""); r.DriverID != best.ID {
		t.Fatalf("got driver %s, want the highest rated %s", r.DriverID, best.ID)
	}
}
