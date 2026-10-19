package pricing_test

import (
	"errors"
	"testing"

	"ridehailing/internal/domain"
	"ridehailing/internal/pricing"
)

func newCalculator(t *testing.T) *pricing.Calculator {
	t.Helper()
	c, err := pricing.NewCalculator(pricing.DefaultTariffs())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func calculate(t *testing.T, req domain.FareRequest) domain.Fare {
	t.Helper()
	if req.Surge == 0 {
		req.Surge = 100
	}
	fare, err := newCalculator(t).Calculate(req)
	if err != nil {
		t.Fatalf("Calculate(%+v): %v", req, err)
	}
	return fare
}

func percent(t *testing.T, pct int) *domain.Coupon {
	t.Helper()
	c, err := domain.NewPercentCoupon("P", pct, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &c
}

func flat(t *testing.T, amount domain.Paise) *domain.Coupon {
	t.Helper()
	c, err := domain.NewFlatCoupon("F", amount, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &c
}

func TestHatchbackTiersAndMinimumFare(t *testing.T) {
	tests := []struct {
		distance       domain.Metres
		metered, total domain.Paise
		minFareApplied bool
	}{
		{0, 0, 5000, true},
		{1000, 1000, 5000, true},
		{2000, 2000, 5000, true},
		{2001, 2001, 5000, true},
		{5000, 4400, 5000, true},
		{6000, 4900, 5000, true},
		{7000, 5400, 5400, false},
		{12345, 8073, 8073, false},
	}
	for _, tc := range tests {
		fare := calculate(t, domain.FareRequest{CarType: domain.Hatchback, Distance: tc.distance})
		if fare.Metered != tc.metered || fare.Total != tc.total || fare.MinFareApplied != tc.minFareApplied {
			t.Errorf("%d m: got metered=%d total=%d minFare=%v, want %d %d %v",
				tc.distance, fare.Metered, fare.Total, fare.MinFareApplied, tc.metered, tc.total, tc.minFareApplied)
		}
	}
}

func TestSedanHasItsOwnRates(t *testing.T) {
	tests := []struct {
		distance       domain.Metres
		metered, total domain.Paise
	}{
		{1000, 1500, 8000},
		{7000, 8200, 8200},
	}
	for _, tc := range tests {
		fare := calculate(t, domain.FareRequest{CarType: domain.Sedan, Distance: tc.distance})
		if fare.Metered != tc.metered || fare.Total != tc.total {
			t.Errorf("sedan %d m: got metered=%d total=%d, want %d %d", tc.distance, fare.Metered, fare.Total, tc.metered, tc.total)
		}
	}
	hatch := calculate(t, domain.FareRequest{CarType: domain.Hatchback, Distance: 7000})
	sedan := calculate(t, domain.FareRequest{CarType: domain.Sedan, Distance: 7000})
	if hatch.Total >= sedan.Total {
		t.Errorf("sedan (%d) should cost more than hatchback (%d) over the same distance", sedan.Total, hatch.Total)
	}
}

func TestSurgeAppliesAfterMinimumFareAndRoundsHalfUp(t *testing.T) {
	tests := []struct {
		distance domain.Metres
		surge    domain.Multiplier
		surged   domain.Paise
	}{
		{7000, 100, 5400},
		{7000, 125, 6750},
		{7000, 150, 8100},
		{7000, 200, 10800},
		{1000, 150, 7500},
		{7002, 125, 6751},
		{7004, 125, 6753},
	}
	for _, tc := range tests {
		fare := calculate(t, domain.FareRequest{CarType: domain.Hatchback, Distance: tc.distance, Surge: tc.surge})
		if fare.Surged != tc.surged || fare.Total != tc.surged || fare.Surge != tc.surge {
			t.Errorf("%d m at %dx: got surged=%d total=%d, want %d", tc.distance, tc.surge, fare.Surged, fare.Total, tc.surged)
		}
	}
}

func TestCoupons(t *testing.T) {
	tests := []struct {
		name            string
		distance        domain.Metres
		surge           domain.Multiplier
		coupon          *domain.Coupon
		discount, total domain.Paise
	}{
		{"percent", 7000, 100, percent(t, 20), 1080, 4320},
		{"percent on surged amount", 7000, 125, percent(t, 20), 1350, 5400},
		{"percent rounds half up", 7004, 100, percent(t, 25), 1351, 4051},
		{"percent on minimum fare", 1000, 100, percent(t, 50), 2500, 2500},
		{"flat", 7000, 100, flat(t, 1000), 1000, 4400},
		{"flat larger than fare floors at zero", 7000, 100, flat(t, 10000), 5400, 0},
		{"no coupon", 7000, 100, nil, 0, 5400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fare := calculate(t, domain.FareRequest{CarType: domain.Hatchback, Distance: tc.distance, Surge: tc.surge, Coupon: tc.coupon})
			if fare.Discount != tc.discount || fare.Total != tc.total {
				t.Fatalf("got discount=%d total=%d, want %d %d", fare.Discount, fare.Total, tc.discount, tc.total)
			}
		})
	}
}

func TestFareBreakdown(t *testing.T) {
	got := calculate(t, domain.FareRequest{CarType: domain.Hatchback, Distance: 7000, Surge: 125, Coupon: percent(t, 20)})
	want := domain.Fare{Distance: 7000, Metered: 5400, MinFareApplied: false, Base: 5400, Surge: 125, Surged: 6750, Discount: 1350, Total: 5400}
	if got != want {
		t.Fatalf("breakdown = %+v, want %+v", got, want)
	}
}

func TestCalculateRejectsBadInput(t *testing.T) {
	bogus := percent(t, 10)
	bogus.Kind = "BOGUS"
	tests := []struct {
		name string
		req  domain.FareRequest
	}{
		{"unknown car type", domain.FareRequest{CarType: "SUV", Distance: 1000, Surge: 100}},
		{"negative distance", domain.FareRequest{CarType: domain.Hatchback, Distance: -1, Surge: 100}},
		{"surge below 1x", domain.FareRequest{CarType: domain.Hatchback, Distance: 1000, Surge: 99}},
		{"unknown coupon kind", domain.FareRequest{CarType: domain.Hatchback, Distance: 1000, Surge: 100, Coupon: bogus}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newCalculator(t).Calculate(tc.req); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestNewCalculatorRejectsBrokenTariffs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[domain.CarType]pricing.Tariff)
	}{
		{"missing car type", func(m map[domain.CarType]pricing.Tariff) { delete(m, domain.Sedan) }},
		{"no tiers", func(m map[domain.CarType]pricing.Tariff) { m[domain.Sedan] = pricing.Tariff{MinFare: 1} }},
		{"last tier closed", func(m map[domain.CarType]pricing.Tariff) {
			m[domain.Sedan] = pricing.Tariff{Tiers: []pricing.Tier{{UpTo: 2000, RatePerKm: 100}}}
		}},
		{"open-ended tier before the last", func(m map[domain.CarType]pricing.Tariff) {
			m[domain.Sedan] = pricing.Tariff{Tiers: []pricing.Tier{{UpTo: 0, RatePerKm: 100}, {UpTo: 0, RatePerKm: 50}}}
		}},
		{"tiers not ascending", func(m map[domain.CarType]pricing.Tariff) {
			m[domain.Sedan] = pricing.Tariff{Tiers: []pricing.Tier{{UpTo: 5000, RatePerKm: 100}, {UpTo: 2000, RatePerKm: 50}, {UpTo: 0, RatePerKm: 10}}}
		}},
		{"negative rate", func(m map[domain.CarType]pricing.Tariff) {
			m[domain.Sedan] = pricing.Tariff{Tiers: []pricing.Tier{{UpTo: 0, RatePerKm: -1}}}
		}},
		{"negative minimum fare", func(m map[domain.CarType]pricing.Tariff) {
			m[domain.Sedan] = pricing.Tariff{MinFare: -1, Tiers: []pricing.Tier{{UpTo: 0, RatePerKm: 1}}}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tariffs := pricing.DefaultTariffs()
			tc.mutate(tariffs)
			if _, err := pricing.NewCalculator(tariffs); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestDefaultTariffsCoverEveryCarType(t *testing.T) {
	tariffs := pricing.DefaultTariffs()
	for _, ct := range domain.AllCarTypes() {
		if _, ok := tariffs[ct]; !ok {
			t.Errorf("no tariff for %s", ct)
		}
	}
}
