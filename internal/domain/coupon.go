package domain

import (
	"fmt"
	"strings"
	"time"
)

type Coupon struct {
	Code      string
	Kind      CouponKind
	Percent   int   // CouponPercent only
	Amount    Paise // CouponFlat only
	ExpiresAt *time.Time
}

func NewPercentCoupon(code string, percent int, expiresAt *time.Time) (Coupon, error) {
	code, err := requireCouponCode(code)
	if err != nil {
		return Coupon{}, err
	}
	if percent < 1 || percent > 100 {
		return Coupon{}, fmt.Errorf("%w: percent %d must be between 1 and 100", ErrValidation, percent)
	}
	return Coupon{Code: code, Kind: CouponPercent, Percent: percent, ExpiresAt: cloneTime(expiresAt)}, nil
}

func NewFlatCoupon(code string, amount Paise, expiresAt *time.Time) (Coupon, error) {
	code, err := requireCouponCode(code)
	if err != nil {
		return Coupon{}, err
	}
	if amount < 1 {
		return Coupon{}, fmt.Errorf("%w: amount %d must be positive", ErrValidation, amount)
	}
	return Coupon{Code: code, Kind: CouponFlat, Amount: amount, ExpiresAt: cloneTime(expiresAt)}, nil
}

func NormalizeCouponCode(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

func requireCouponCode(code string) (string, error) {
	code = NormalizeCouponCode(code)
	if code == "" {
		return "", fmt.Errorf("%w: coupon code is required", ErrValidation)
	}
	return code, nil
}

// ValidAt is false from the expiry instant onwards.
func (c Coupon) ValidAt(t time.Time) bool { return c.ExpiresAt == nil || t.Before(*c.ExpiresAt) }

func (c Coupon) Clone() Coupon {
	c.ExpiresAt = cloneTime(c.ExpiresAt)
	return c
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}
