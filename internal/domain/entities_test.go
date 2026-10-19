package domain_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"ridehailing/internal/domain"
)

var (
	now    = time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)
	pickup = domain.Location{Lat: 12.9716, Lng: 77.5946}
)

func TestNewUser(t *testing.T) {
	u, err := domain.NewUser("  Asha  ", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Name != "Asha" || u.ID == "" || !u.CreatedAt.Equal(now) {
		t.Fatalf("unexpected user: %+v", u)
	}
	if _, err := domain.NewUser("   ", now); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("blank name error = %v, want ErrValidation", err)
	}
}

func TestNewDriver(t *testing.T) {
	d, err := domain.NewDriver("Ravi", domain.Sedan, pickup, 4.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != domain.DriverAvailable || d.ID == "" || d.CarType != domain.Sedan || d.Rating != 4.5 {
		t.Fatalf("unexpected driver: %+v", d)
	}

	bad := []struct {
		name    string
		drv     string
		carType domain.CarType
		loc     domain.Location
		rating  float64
	}{
		{"blank name", " ", domain.Sedan, pickup, 4},
		{"unknown car type", "Ravi", "SUV", pickup, 4},
		{"bad location", "Ravi", domain.Sedan, domain.Location{Lat: 91}, 4},
		{"rating below zero", "Ravi", domain.Sedan, pickup, -0.1},
		{"rating above five", "Ravi", domain.Sedan, pickup, 5.1},
		{"rating nan", "Ravi", domain.Sedan, pickup, math.NaN()},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := domain.NewDriver(tc.drv, tc.carType, tc.loc, tc.rating); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestNewPercentCoupon(t *testing.T) {
	c, err := domain.NewPercentCoupon(" save20 ", 20, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Code != "SAVE20" || c.Kind != domain.CouponPercent || c.Percent != 20 {
		t.Fatalf("unexpected coupon: %+v", c)
	}
	for _, pct := range []int{0, 101, -5} {
		if _, err := domain.NewPercentCoupon("X", pct, nil); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("percent %d: error = %v, want ErrValidation", pct, err)
		}
	}
	if _, err := domain.NewPercentCoupon("  ", 10, nil); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("blank code: error = %v, want ErrValidation", err)
	}
}

func TestNewFlatCoupon(t *testing.T) {
	exp := now.Add(time.Hour)
	c, err := domain.NewFlatCoupon("flat50", 5000, &exp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Code != "FLAT50" || c.Kind != domain.CouponFlat || c.Amount != 5000 || !c.ExpiresAt.Equal(exp) {
		t.Fatalf("unexpected coupon: %+v", c)
	}
	for _, amt := range []domain.Paise{0, -1} {
		if _, err := domain.NewFlatCoupon("X", amt, nil); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("amount %d: error = %v, want ErrValidation", amt, err)
		}
	}
	if _, err := domain.NewFlatCoupon(" ", 100, nil); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("blank code: error = %v, want ErrValidation", err)
	}
}

func TestCouponValidAt(t *testing.T) {
	exp := now
	tests := []struct {
		name string
		exp  *time.Time
		at   time.Time
		want bool
	}{
		{"no expiry", nil, now.Add(24 * time.Hour), true},
		{"before expiry", &exp, now.Add(-time.Second), true},
		{"at expiry", &exp, now, false},
		{"after expiry", &exp, now.Add(time.Second), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := domain.NewPercentCoupon("C", 10, tc.exp)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.ValidAt(tc.at); got != tc.want {
				t.Fatalf("ValidAt = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCouponCloneIsIndependent(t *testing.T) {
	exp := now
	c, _ := domain.NewPercentCoupon("C", 10, &exp)
	clone := c.Clone()
	*clone.ExpiresAt = now.Add(time.Hour)
	if !c.ExpiresAt.Equal(now) {
		t.Fatal("clone shares ExpiresAt with the original")
	}
}

func newRide(t *testing.T, driverType domain.CarType, coupon *domain.Coupon) domain.Ride {
	t.Helper()
	d, err := domain.NewDriver("Ravi", driverType, pickup, 4.5)
	if err != nil {
		t.Fatal(err)
	}
	return domain.NewRide("usr_1", d, domain.Hatchback, pickup, 125, coupon, now)
}

func TestNewRide(t *testing.T) {
	coupon, _ := domain.NewPercentCoupon("SAVE20", 20, nil)
	r := newRide(t, domain.Sedan, &coupon)
	if r.Status != domain.RideOngoing || r.ID == "" || r.UserID != "usr_1" || r.DriverID == "" {
		t.Fatalf("unexpected ride: %+v", r)
	}
	if r.RequestedCarType != domain.Hatchback || r.AssignedCarType != domain.Sedan || !r.Upgraded {
		t.Fatalf("upgrade not recorded: %+v", r)
	}
	if r.Surge != 125 || !r.CreatedAt.Equal(now) || r.Drop != nil || r.Fare != nil || r.EndedAt != nil {
		t.Fatalf("unexpected ride fields: %+v", r)
	}
	coupon.Percent = 50
	if r.Coupon.Percent != 20 {
		t.Fatal("ride coupon is not a snapshot")
	}
	if plain := newRide(t, domain.Hatchback, nil); plain.Upgraded || plain.Coupon != nil {
		t.Fatalf("plain ride should not be upgraded or carry a coupon: %+v", plain)
	}
}

func TestRideComplete(t *testing.T) {
	r := newRide(t, domain.Hatchback, nil)
	drop := domain.Location{Lat: 12.93, Lng: 77.62}
	fare := domain.Fare{Total: 5400}
	ended := now.Add(20 * time.Minute)

	if err := r.Complete(drop, fare, ended); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != domain.RideCompleted || *r.Drop != drop || r.Fare.Total != 5400 || !r.EndedAt.Equal(ended) {
		t.Fatalf("unexpected ride after complete: %+v", r)
	}

	err := r.Complete(drop, domain.Fare{Total: 1}, ended.Add(time.Minute))
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("second complete error = %v, want ErrInvalidTransition", err)
	}
	if r.Fare.Total != 5400 || !r.EndedAt.Equal(ended) {
		t.Fatal("failed transition must not change the ride")
	}
	if err := r.Cancel(0, ended); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("cancel after complete error = %v, want ErrInvalidTransition", err)
	}
}

func TestRideCancel(t *testing.T) {
	r := newRide(t, domain.Hatchback, nil)
	at := now.Add(5 * time.Minute)
	if err := r.Cancel(2000, at); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != domain.RideCancelled || r.CancellationFee != 2000 || !r.EndedAt.Equal(at) {
		t.Fatalf("unexpected ride after cancel: %+v", r)
	}
	if err := r.Cancel(0, at); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("second cancel error = %v, want ErrInvalidTransition", err)
	}
	if err := r.Complete(pickup, domain.Fare{}, at); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("complete after cancel error = %v, want ErrInvalidTransition", err)
	}
	if r.CancellationFee != 2000 {
		t.Fatal("failed transition must not change the fee")
	}
}

func TestRideCloneIsIndependent(t *testing.T) {
	exp := now.Add(time.Hour)
	coupon, _ := domain.NewPercentCoupon("SAVE20", 20, &exp)
	r := newRide(t, domain.Hatchback, &coupon)
	if err := r.Complete(domain.Location{Lat: 1}, domain.Fare{Total: 100}, now); err != nil {
		t.Fatal(err)
	}

	c := r.Clone()
	c.Drop.Lat = 2
	c.Coupon.Percent = 99
	*c.Coupon.ExpiresAt = now
	c.Fare.Total = 1
	*c.EndedAt = now.Add(time.Hour)

	if r.Drop.Lat != 1 || r.Coupon.Percent != 20 || !r.Coupon.ExpiresAt.Equal(exp) || r.Fare.Total != 100 || !r.EndedAt.Equal(now) {
		t.Fatalf("clone shares memory with the original: %+v", r)
	}
	if bare := newRide(t, domain.Hatchback, nil).Clone(); bare.Drop != nil || bare.Coupon != nil || bare.Fare != nil || bare.EndedAt != nil {
		t.Fatal("clone of a fresh ride should keep nil fields nil")
	}
}
