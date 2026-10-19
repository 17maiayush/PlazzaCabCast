// Package domain holds the entities, value types and sentinel errors every other package builds on.
// It imports nothing from the rest of the project.
package domain

import (
	"fmt"
	"slices"
	"strings"
)

type CarType string

const (
	Hatchback CarType = "HATCHBACK"
	Sedan     CarType = "SEDAN"
)

// AllCarTypes is the list every tariff table and validator must cover.
func AllCarTypes() []CarType { return []CarType{Hatchback, Sedan} }

// upgrades lists, in order, the car types a rider may be moved to at no extra cost.
var upgrades = map[CarType][]CarType{
	Hatchback: {Sedan},
}

func UpgradesFor(c CarType) []CarType { return slices.Clone(upgrades[c]) }

func (c CarType) Valid() bool { return slices.Contains(AllCarTypes(), c) }

func ParseCarType(s string) (CarType, error) {
	c := CarType(strings.ToUpper(strings.TrimSpace(s)))
	if !c.Valid() {
		return "", fmt.Errorf("%w: unknown car type %q", ErrValidation, s)
	}
	return c, nil
}

// Paise is money. Fares are integer paise end to end so rounding happens where we say it does.
type Paise int64

func (p Paise) INR() string { return fmt.Sprintf("%d.%02d", p/100, p%100) }

type Metres int64

// Multiplier is a percentage: 100 is 1.00x, 150 is 1.50x.
type Multiplier int

func (m Multiplier) Float() float64 { return float64(m) / 100 }

type DriverStatus string

const (
	DriverAvailable DriverStatus = "AVAILABLE"
	DriverBusy      DriverStatus = "BUSY"
)

type RideStatus string

const (
	RideOngoing   RideStatus = "ONGOING"
	RideCompleted RideStatus = "COMPLETED"
	RideCancelled RideStatus = "CANCELLED"
)

func ParseRideStatus(s string) (RideStatus, error) {
	switch st := RideStatus(strings.ToUpper(strings.TrimSpace(s))); st {
	case RideOngoing, RideCompleted, RideCancelled:
		return st, nil
	}
	return "", fmt.Errorf("%w: unknown ride status %q", ErrValidation, s)
}

type CouponKind string

const (
	CouponPercent CouponKind = "PERCENT"
	CouponFlat    CouponKind = "FLAT"
)
