// Package service holds the use cases. It declares the interfaces it needs; the adapters in
// store, pricing, matching and cancellation satisfy them without importing this package.
package service

import (
	"time"

	"ridehailing/internal/domain"
)

type Clock func() time.Time

type UserRepo interface {
	Save(domain.User) error
	Get(id string) (domain.User, error)
	List() []domain.User
}

type DriverRepo interface {
	Save(domain.Driver) error
	Get(id string) (domain.Driver, error)
	List() []domain.Driver
	SetLocation(id string, loc domain.Location) error
	AvailableWithin(center domain.Location, radius domain.Metres, carType domain.CarType) []domain.Driver
	CountAvailableWithin(center domain.Location, radius domain.Metres) int
	Reserve(id string) error
	Release(id string) error
}

type RideRepo interface {
	Save(domain.Ride) error
	Get(id string) (domain.Ride, error)
	Update(id string, mutate func(*domain.Ride) error) (domain.Ride, error)
	ListByUser(userID string, status *domain.RideStatus) []domain.Ride
	ListByDriver(driverID string, status *domain.RideStatus) []domain.Ride
	CountOngoingWithin(center domain.Location, radius domain.Metres) int
}

type CouponRepo interface {
	Save(domain.Coupon) error
	Get(code string) (domain.Coupon, error)
	List() []domain.Coupon
	Delete(code string) error
}

type FareCalculator interface {
	Calculate(domain.FareRequest) (domain.Fare, error)
}

// MatchStrategy orders candidates; the booking loop reserves them in that order.
type MatchStrategy interface {
	Rank(candidates []domain.Driver, pickup domain.Location) []domain.Driver
}

type SurgeStrategy interface {
	Multiplier(domain.SurgeInput) domain.Multiplier
}

type CancellationPolicy interface {
	Fee(ride domain.Ride, now time.Time) domain.Paise
}
