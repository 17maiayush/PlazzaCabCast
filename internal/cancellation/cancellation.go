// Package cancellation decides what a rider pays for cancelling.
package cancellation

import (
	"time"

	"ridehailing/internal/domain"
)

// GracePolicy is free within Grace of booking and a flat Amount after that.
type GracePolicy struct {
	Grace  time.Duration
	Amount domain.Paise
}

func (p GracePolicy) Fee(ride domain.Ride, now time.Time) domain.Paise {
	if now.Sub(ride.CreatedAt) <= p.Grace {
		return 0
	}
	return p.Amount
}
