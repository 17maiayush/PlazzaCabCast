package domain

import (
	"fmt"
	"time"
)

type Ride struct {
	ID               string
	UserID           string
	DriverID         string
	RequestedCarType CarType
	AssignedCarType  CarType
	Upgraded         bool
	Pickup           Location
	Drop             *Location
	Status           RideStatus
	Surge            Multiplier // locked at booking
	Coupon           *Coupon    // snapshot at booking
	Fare             *Fare      // set on Complete
	CancellationFee  Paise      // set on Cancel
	CreatedAt        time.Time
	EndedAt          *time.Time
}

// NewRide starts a ride as ONGOING: there is no driver accept/reject step in this system.
func NewRide(userID string, driver Driver, requested CarType, pickup Location, surge Multiplier, coupon *Coupon, now time.Time) Ride {
	r := Ride{
		ID:               NewID("ride"),
		UserID:           userID,
		DriverID:         driver.ID,
		RequestedCarType: requested,
		AssignedCarType:  driver.CarType,
		Upgraded:         driver.CarType != requested,
		Pickup:           pickup,
		Status:           RideOngoing,
		Surge:            surge,
		CreatedAt:        now,
	}
	if coupon != nil {
		c := coupon.Clone()
		r.Coupon = &c
	}
	return r
}

func (r *Ride) Complete(drop Location, fare Fare, at time.Time) error {
	if err := r.requireOngoing(); err != nil {
		return err
	}
	r.Status = RideCompleted
	r.Drop = &drop
	r.Fare = &fare
	r.EndedAt = &at
	return nil
}

func (r *Ride) Cancel(fee Paise, at time.Time) error {
	if err := r.requireOngoing(); err != nil {
		return err
	}
	r.Status = RideCancelled
	r.CancellationFee = fee
	r.EndedAt = &at
	return nil
}

func (r *Ride) requireOngoing() error {
	if r.Status != RideOngoing {
		return fmt.Errorf("%w: ride %s is %s", ErrInvalidTransition, r.ID, r.Status)
	}
	return nil
}

// Clone deep-copies the pointer fields so a stored ride can never be changed through a returned copy.
func (r Ride) Clone() Ride {
	if r.Drop != nil {
		d := *r.Drop
		r.Drop = &d
	}
	if r.Coupon != nil {
		c := r.Coupon.Clone()
		r.Coupon = &c
	}
	if r.Fare != nil {
		f := *r.Fare
		r.Fare = &f
	}
	r.EndedAt = cloneTime(r.EndedAt)
	return r
}
