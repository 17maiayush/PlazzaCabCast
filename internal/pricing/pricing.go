// Package pricing turns a distance into a fare: marginal tiers, minimum fare, surge, then coupon, in that order.
package pricing

import (
	"errors"
	"fmt"
	"maps"

	"ridehailing/internal/domain"
)

type Tier struct {
	UpTo      domain.Metres // 0 means open-ended; only the last tier may be
	RatePerKm domain.Paise
}

type Tariff struct {
	MinFare domain.Paise
	Tiers   []Tier
}

// DefaultTariffs uses the assignment's example for Hatchback; Sedan is roughly 1.5x of it.
func DefaultTariffs() map[domain.CarType]Tariff {
	return map[domain.CarType]Tariff{
		domain.Hatchback: {MinFare: 5000, Tiers: []Tier{
			{UpTo: 2000, RatePerKm: 1000},
			{UpTo: 5000, RatePerKm: 800},
			{UpTo: 0, RatePerKm: 500},
		}},
		domain.Sedan: {MinFare: 8000, Tiers: []Tier{
			{UpTo: 2000, RatePerKm: 1500},
			{UpTo: 5000, RatePerKm: 1200},
			{UpTo: 0, RatePerKm: 800},
		}},
	}
}

type Calculator struct {
	tariffs map[domain.CarType]Tariff
}

// NewCalculator fails fast so a car type without a tariff can never reach a booking.
func NewCalculator(tariffs map[domain.CarType]Tariff) (*Calculator, error) {
	for _, ct := range domain.AllCarTypes() {
		t, ok := tariffs[ct]
		if !ok {
			return nil, fmt.Errorf("no tariff for %s", ct)
		}
		if err := t.validate(); err != nil {
			return nil, fmt.Errorf("tariff for %s: %w", ct, err)
		}
	}
	return &Calculator{tariffs: maps.Clone(tariffs)}, nil
}

func (t Tariff) validate() error {
	if t.MinFare < 0 {
		return errors.New("minimum fare must not be negative")
	}
	if len(t.Tiers) == 0 {
		return errors.New("at least one tier is required")
	}
	var prev domain.Metres
	for i, tier := range t.Tiers {
		if tier.RatePerKm < 0 {
			return fmt.Errorf("tier %d: rate must not be negative", i)
		}
		if i == len(t.Tiers)-1 {
			if tier.UpTo != 0 {
				return errors.New("the last tier must be open-ended")
			}
			continue
		}
		if tier.UpTo <= prev {
			return fmt.Errorf("tier %d: boundaries must ascend and only the last tier may be open-ended", i)
		}
		prev = tier.UpTo
	}
	return nil
}

func (c *Calculator) Calculate(req domain.FareRequest) (domain.Fare, error) {
	tariff, ok := c.tariffs[req.CarType]
	if !ok {
		return domain.Fare{}, fmt.Errorf("%w: no tariff for car type %q", domain.ErrValidation, req.CarType)
	}
	if req.Distance < 0 {
		return domain.Fare{}, fmt.Errorf("%w: distance must not be negative", domain.ErrValidation)
	}
	if req.Surge < 100 {
		return domain.Fare{}, fmt.Errorf("%w: surge multiplier %d is below 1.00x", domain.ErrValidation, req.Surge)
	}

	fare := domain.Fare{Distance: req.Distance, Surge: req.Surge}
	fare.Metered = tariff.metered(req.Distance)
	fare.Base = max(fare.Metered, tariff.MinFare)
	fare.MinFareApplied = fare.Metered < tariff.MinFare
	fare.Surged = domain.Paise(roundHalfUp(int64(fare.Base)*int64(req.Surge), 100))

	discount, err := discountFor(req.Coupon, fare.Surged)
	if err != nil {
		return domain.Fare{}, err
	}
	fare.Discount = min(discount, fare.Surged)
	fare.Total = fare.Surged - fare.Discount
	return fare, nil
}

// metered sums each tier's share of the distance in paise-metres and rounds once at the end.
func (t Tariff) metered(d domain.Metres) domain.Paise {
	var sum int64
	var lower domain.Metres
	for _, tier := range t.Tiers {
		upper := tier.UpTo
		if upper == 0 || upper > d {
			upper = d
		}
		if upper > lower {
			sum += int64(upper-lower) * int64(tier.RatePerKm)
			lower = upper
		}
	}
	return domain.Paise(roundHalfUp(sum, 1000))
}

func discountFor(c *domain.Coupon, amount domain.Paise) (domain.Paise, error) {
	if c == nil {
		return 0, nil
	}
	switch c.Kind {
	case domain.CouponPercent:
		return domain.Paise(roundHalfUp(int64(amount)*int64(c.Percent), 100)), nil
	case domain.CouponFlat:
		return c.Amount, nil
	}
	return 0, fmt.Errorf("%w: unknown coupon kind %q", domain.ErrValidation, c.Kind)
}

func roundHalfUp(a, b int64) int64 { return (a + b/2) / b }
