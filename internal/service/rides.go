package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"ridehailing/internal/domain"
)

type RidesDeps struct {
	Users        UserRepo
	Drivers      DriverRepo
	Rides        RideRepo
	Coupons      CouponRepo
	Fares        FareCalculator
	Match        MatchStrategy
	Surge        SurgeStrategy
	Cancellation CancellationPolicy
	Radius       domain.Metres
	Now          Clock
}

type Rides struct {
	deps RidesDeps
}

func NewRides(deps RidesDeps) *Rides { return &Rides{deps: deps} }

type BookRequest struct {
	UserID     string
	CarType    domain.CarType
	Pickup     domain.Location
	CouponCode string
}

func (s *Rides) Book(req BookRequest) (domain.Ride, error) {
	if err := validateTrip(req.CarType, req.Pickup); err != nil {
		return domain.Ride{}, err
	}
	if _, err := s.deps.Users.Get(req.UserID); err != nil {
		return domain.Ride{}, err
	}
	now := s.deps.Now()
	coupon, err := s.coupon(req.CouponCode, now)
	if err != nil {
		return domain.Ride{}, err
	}

	// Surge is read before any reservation so the supply count still includes the driver about to be taken.
	surge := s.surgeAt(req.Pickup)

	types := searchOrder(req.CarType)
	candidates, _ := s.candidates(types, req.Pickup)
	if len(candidates) == 0 {
		return domain.Ride{}, fmt.Errorf("%w: no %s driver within %d m of the pickup",
			domain.ErrNoDriverAvailable, joinTypes(types), s.deps.Radius)
	}

	for _, d := range s.deps.Match.Rank(candidates, req.Pickup) {
		ride, err := s.reserveAndSave(d.ID, func() domain.Ride {
			return domain.NewRide(req.UserID, d, req.CarType, req.Pickup, surge, coupon, now)
		})
		if errors.Is(err, domain.ErrDriverBusy) {
			continue // another booking won this driver between the query and the reserve
		}
		return ride, err
	}
	return domain.Ride{}, fmt.Errorf("%w: every candidate was taken while matching", domain.ErrNoDriverAvailable)
}

// candidates tries the types in order and stops at the first non-empty set, so a rider only gets
// an upgrade when nothing of the requested type is available, never because the upgrade is nearer.
func (s *Rides) candidates(types []domain.CarType, pickup domain.Location) ([]domain.Driver, domain.CarType) {
	for _, ct := range types {
		if found := s.deps.Drivers.AvailableWithin(pickup, s.deps.Radius, ct); len(found) > 0 {
			return found, ct
		}
	}
	return nil, ""
}

func searchOrder(requested domain.CarType) []domain.CarType {
	return append([]domain.CarType{requested}, domain.UpgradesFor(requested)...)
}

func (s *Rides) surgeAt(pickup domain.Location) domain.Multiplier {
	return s.deps.Surge.Multiplier(domain.SurgeInput{
		Demand: s.deps.Rides.CountOngoingWithin(pickup, s.deps.Radius),
		Supply: s.deps.Drivers.CountAvailableWithin(pickup, s.deps.Radius),
	})
}

func validateTrip(carType domain.CarType, pickup domain.Location) error {
	if !carType.Valid() {
		return fmt.Errorf("%w: unknown car type %q", domain.ErrValidation, carType)
	}
	return pickup.Validate()
}

type EstimateRequest struct {
	CarType    domain.CarType
	Pickup     domain.Location
	Drop       domain.Location
	CouponCode string
}

// Estimate is a quote, not a hold: nothing is reserved, so surge or availability can change before booking.
type Estimate struct {
	Fare            domain.Fare
	Coupon          *domain.Coupon
	AssignedCarType domain.CarType // empty when no driver is in range
}

func (s *Rides) Estimate(req EstimateRequest) (Estimate, error) {
	if err := validateTrip(req.CarType, req.Pickup); err != nil {
		return Estimate{}, err
	}
	if err := req.Drop.Validate(); err != nil {
		return Estimate{}, err
	}
	coupon, err := s.coupon(req.CouponCode, s.deps.Now())
	if err != nil {
		return Estimate{}, err
	}
	fare, err := s.deps.Fares.Calculate(domain.FareRequest{
		CarType:  req.CarType,
		Distance: req.Pickup.DistanceTo(req.Drop),
		Surge:    s.surgeAt(req.Pickup),
		Coupon:   coupon,
	})
	if err != nil {
		return Estimate{}, err
	}
	_, assigned := s.candidates(searchOrder(req.CarType), req.Pickup)
	return Estimate{Fare: fare, Coupon: coupon, AssignedCarType: assigned}, nil
}

// reserveAndSave is the booking's critical section. The deferred release is the compensation for
// anything that goes wrong after the reservation, including a panic in the ride store.
func (s *Rides) reserveAndSave(driverID string, build func() domain.Ride) (ride domain.Ride, err error) {
	if err := s.deps.Drivers.Reserve(driverID); err != nil {
		return domain.Ride{}, err
	}
	committed := false
	defer func() {
		if !committed {
			err = errors.Join(err, s.deps.Drivers.Release(driverID))
		}
	}()
	ride = build()
	if err := s.deps.Rides.Save(ride); err != nil {
		return domain.Ride{}, err
	}
	committed = true
	return ride, nil
}

func (s *Rides) coupon(code string, now time.Time) (*domain.Coupon, error) {
	code = domain.NormalizeCouponCode(code)
	if code == "" {
		return nil, nil
	}
	c, err := s.deps.Coupons.Get(code)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidCoupon, code)
	}
	if !c.ValidAt(now) {
		return nil, fmt.Errorf("%w: %s has expired", domain.ErrInvalidCoupon, code)
	}
	return &c, nil
}

func (s *Rides) End(id string, drop domain.Location) (domain.Ride, error) {
	if err := drop.Validate(); err != nil {
		return domain.Ride{}, err
	}
	ride, err := s.deps.Rides.Get(id)
	if err != nil {
		return domain.Ride{}, err
	}
	fare, err := s.deps.Fares.Calculate(domain.FareRequest{
		CarType:  ride.RequestedCarType, // the requested type is what makes an upgrade free
		Distance: ride.Pickup.DistanceTo(drop),
		Surge:    ride.Surge,
		Coupon:   ride.Coupon,
	})
	if err != nil {
		return domain.Ride{}, err
	}
	now := s.deps.Now()
	updated, err := s.deps.Rides.Update(id, func(r *domain.Ride) error { return r.Complete(drop, fare, now) })
	if err != nil {
		return domain.Ride{}, err
	}
	return updated, s.releaseDriver(ride.DriverID, &drop)
}

func (s *Rides) Cancel(id string) (domain.Ride, error) {
	ride, err := s.deps.Rides.Get(id)
	if err != nil {
		return domain.Ride{}, err
	}
	now := s.deps.Now()
	fee := s.deps.Cancellation.Fee(ride, now)
	updated, err := s.deps.Rides.Update(id, func(r *domain.Ride) error { return r.Cancel(fee, now) })
	if err != nil {
		return domain.Ride{}, err
	}
	return updated, s.releaseDriver(ride.DriverID, nil)
}

// releaseDriver runs only after the ride's transition has been saved. A ride that was already
// ended never reaches it, because its driver may by now be serving someone else.
func (s *Rides) releaseDriver(id string, at *domain.Location) error {
	if err := s.deps.Drivers.Release(id); err != nil {
		return err
	}
	if at == nil {
		return nil
	}
	return s.deps.Drivers.SetLocation(id, *at)
}

func (s *Rides) ForUser(userID string, status *domain.RideStatus) ([]domain.Ride, error) {
	if _, err := s.deps.Users.Get(userID); err != nil {
		return nil, err
	}
	return s.deps.Rides.ListByUser(userID, status), nil
}

func (s *Rides) ForDriver(driverID string, status *domain.RideStatus) ([]domain.Ride, error) {
	if _, err := s.deps.Drivers.Get(driverID); err != nil {
		return nil, err
	}
	return s.deps.Rides.ListByDriver(driverID, status), nil
}

func joinTypes(types []domain.CarType) string {
	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}
	return strings.Join(names, " or ")
}
