package domain

import (
	"fmt"
	"math"
)

// Driver and cab are one entity here; a location update on the cab is a location update on the driver.
type Driver struct {
	ID       string
	Name     string
	CarType  CarType
	Location Location
	Rating   float64
	Status   DriverStatus
}

func NewDriver(name string, carType CarType, loc Location, rating float64) (Driver, error) {
	name, err := requireName(name)
	if err != nil {
		return Driver{}, err
	}
	if !carType.Valid() {
		return Driver{}, fmt.Errorf("%w: unknown car type %q", ErrValidation, carType)
	}
	if err := loc.Validate(); err != nil {
		return Driver{}, err
	}
	if math.IsNaN(rating) || rating < 0 || rating > 5 {
		return Driver{}, fmt.Errorf("%w: rating %v must be between 0 and 5", ErrValidation, rating)
	}
	return Driver{
		ID:       NewID("drv"),
		Name:     name,
		CarType:  carType,
		Location: loc,
		Rating:   rating,
		Status:   DriverAvailable,
	}, nil
}
