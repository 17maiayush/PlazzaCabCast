package service_test

import (
	"errors"
	"testing"

	"ridehailing/internal/domain"
	"ridehailing/internal/pricing"
	"ridehailing/internal/service"
)

func estimate(h *harness, carType domain.CarType, coupon string) (service.Estimate, error) {
	return h.rides.Estimate(service.EstimateRequest{CarType: carType, Pickup: pickup, Drop: sevenKm, CouponCode: coupon})
}

func TestEstimateQuotesWithoutReserving(t *testing.T) {
	h := newHarness(t)
	d := h.driver(t, domain.Hatchback, 0.01, 4)

	est, err := estimate(h, domain.Hatchback, "")
	if err != nil {
		t.Fatal(err)
	}
	if est.Fare.Distance != 7000 || est.Fare.Total != 5400 || est.AssignedCarType != domain.Hatchback || est.Coupon != nil {
		t.Fatalf("estimate = %+v", est)
	}
	if h.driverStatus(t, d.ID) != domain.DriverAvailable {
		t.Fatal("an estimate must not reserve a driver")
	}
}

func TestEstimateShowsTheUpgradeAtTheRequestedPrice(t *testing.T) {
	h := newHarness(t)
	h.driver(t, domain.Sedan, 0.01, 4)
	est, err := estimate(h, domain.Hatchback, "")
	if err != nil {
		t.Fatal(err)
	}
	if est.AssignedCarType != domain.Sedan || est.Fare.Total != 5400 {
		t.Fatalf("estimate = %+v, want a sedan at the hatchback price", est)
	}
}

func TestEstimateWithNoDriverStillQuotes(t *testing.T) {
	h := newHarness(t)
	est, err := estimate(h, domain.Sedan, "")
	if err != nil {
		t.Fatal(err)
	}
	if est.AssignedCarType != "" || est.Fare.Total != 8200 {
		t.Fatalf("estimate = %+v, want a sedan quote with no driver", est)
	}
}

func TestEstimateAppliesCouponAndSurge(t *testing.T) {
	h := newHarness(t, func(d *service.RidesDeps) { d.Surge = pricing.DefaultStepSurge() })
	u := h.user(t)
	h.driver(t, domain.Hatchback, 0.01, 4)
	h.driver(t, domain.Hatchback, 0.01, 4)
	h.driver(t, domain.Hatchback, 0.01, 4)
	h.book(t, u.ID, "")
	h.book(t, u.ID, "")
	c, _ := domain.NewPercentCoupon("SAVE20", 20, nil)
	if err := h.coupons.Add(c); err != nil {
		t.Fatal(err)
	}

	est, err := estimate(h, domain.Hatchback, "save20")
	if err != nil {
		t.Fatal(err)
	}
	if est.Fare.Surge != 125 || est.Fare.Discount != 1350 || est.Fare.Total != 5400 || est.Coupon.Code != "SAVE20" {
		t.Fatalf("estimate = %+v, want 1.25x surge then 20%% off", est)
	}
}

func TestEstimateRejections(t *testing.T) {
	h := newHarness(t)
	tests := []struct {
		name string
		req  service.EstimateRequest
		want error
	}{
		{"unknown car type", service.EstimateRequest{CarType: "SUV", Pickup: pickup, Drop: sevenKm}, domain.ErrValidation},
		{"bad pickup", service.EstimateRequest{CarType: domain.Sedan, Pickup: domain.Location{Lat: 91}, Drop: sevenKm}, domain.ErrValidation},
		{"bad drop", service.EstimateRequest{CarType: domain.Sedan, Pickup: pickup, Drop: domain.Location{Lng: 181}}, domain.ErrValidation},
		{"unknown coupon", service.EstimateRequest{CarType: domain.Sedan, Pickup: pickup, Drop: sevenKm, CouponCode: "NOPE"}, domain.ErrInvalidCoupon},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.rides.Estimate(tc.req); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestEstimateSurfacesFareErrors(t *testing.T) {
	boom := errors.New("tariff missing")
	h := newHarness(t, func(d *service.RidesDeps) { d.Fares = failingFares{err: boom} })
	if _, err := estimate(h, domain.Sedan, ""); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the fare failure", err)
	}
}

func TestListsComeFromTheStores(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	d := h.driver(t, domain.Sedan, 0.01, 4)
	c, _ := domain.NewFlatCoupon("FLAT30", 3000, nil)
	if err := h.coupons.Add(c); err != nil {
		t.Fatal(err)
	}
	if got := h.users.List(); len(got) != 1 || got[0].ID != u.ID {
		t.Fatalf("users = %+v", got)
	}
	if got := h.driverOps.List(); len(got) != 1 || got[0].ID != d.ID {
		t.Fatalf("drivers = %+v", got)
	}
	if got := h.coupons.List(); len(got) != 1 || got[0].Code != "FLAT30" {
		t.Fatalf("coupons = %+v", got)
	}
}
