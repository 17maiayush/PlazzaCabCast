package pricing_test

import (
	"testing"

	"ridehailing/internal/domain"
	"ridehailing/internal/pricing"
)

// FuzzCalculate checks the properties every fare must have, whatever the inputs:
// the steps only ever move in one direction, and more distance never costs less.
// `make fuzz` explores beyond the seeds; plain `go test` replays them.
func FuzzCalculate(f *testing.F) {
	f.Add(int64(0), 100, 0, int64(0), true)
	f.Add(int64(7000), 125, 20, int64(0), true)
	f.Add(int64(6200), 200, 0, int64(10000), false)
	f.Add(int64(12345), 150, 100, int64(1), false)

	calc, err := pricing.NewCalculator(pricing.DefaultTariffs())
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, distance int64, surge, percent int, flat int64, sedan bool) {
		req := domain.FareRequest{
			CarType:  domain.Hatchback,
			Distance: domain.Metres(abs(distance) % 10_000_000), // up to 10,000 km keeps int64 far from overflow
			Surge:    domain.Multiplier(100 + abs(surge)%201),
			Coupon:   fuzzCoupon(t, percent, flat),
		}
		if sedan {
			req.CarType = domain.Sedan
		}

		fare, err := calc.Calculate(req)
		if err != nil {
			t.Fatalf("Calculate(%+v): %v", req, err)
		}
		switch {
		case fare.Base < fare.Metered:
			t.Fatalf("base %d below metered %d", fare.Base, fare.Metered)
		case fare.MinFareApplied != (fare.Metered < fare.Base):
			t.Fatalf("MinFareApplied=%v but metered %d, base %d", fare.MinFareApplied, fare.Metered, fare.Base)
		case fare.Surged < fare.Base:
			t.Fatalf("surged %d below base %d", fare.Surged, fare.Base)
		case fare.Discount < 0 || fare.Discount > fare.Surged:
			t.Fatalf("discount %d outside [0, %d]", fare.Discount, fare.Surged)
		case fare.Total != fare.Surged-fare.Discount || fare.Total < 0:
			t.Fatalf("total %d, want surged %d minus discount %d", fare.Total, fare.Surged, fare.Discount)
		}

		longer := req
		longer.Distance++
		next, err := calc.Calculate(longer)
		if err != nil {
			t.Fatal(err)
		}
		if next.Base < fare.Base || next.Surged < fare.Surged {
			t.Fatalf("one more metre got cheaper: %+v then %+v", fare, next)
		}
	})
}

func fuzzCoupon(t *testing.T, percent int, flat int64) *domain.Coupon {
	t.Helper()
	var (
		c   domain.Coupon
		err error
	)
	switch {
	case percent != 0:
		c, err = domain.NewPercentCoupon("FUZZ", 1+abs(percent)%100, nil)
	case flat != 0:
		c, err = domain.NewFlatCoupon("FUZZ", domain.Paise(1+abs(flat)%1_000_000), nil)
	default:
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return &c
}

func abs[T int | int64](v T) T {
	if v < 0 {
		// the most negative value has no positive twin; zero keeps the input valid
		if -v < 0 {
			return 0
		}
		return -v
	}
	return v
}
