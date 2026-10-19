package domain_test

import (
	"errors"
	"math"
	"testing"

	"ridehailing/internal/domain"
)

func TestLocationValidate(t *testing.T) {
	tests := []struct {
		name string
		loc  domain.Location
		ok   bool
	}{
		{"origin", domain.Location{}, true},
		{"bengaluru", domain.Location{Lat: 12.97, Lng: 77.59}, true},
		{"edges", domain.Location{Lat: -90, Lng: 180}, true},
		{"lat too high", domain.Location{Lat: 90.01}, false},
		{"lat too low", domain.Location{Lat: -90.01}, false},
		{"lng too high", domain.Location{Lng: 180.01}, false},
		{"lng too low", domain.Location{Lng: -180.01}, false},
		{"nan lat", domain.Location{Lat: math.NaN()}, false},
		{"nan lng", domain.Location{Lng: math.NaN()}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.loc.Validate()
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.ok && !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestDistanceTo(t *testing.T) {
	origin := domain.Location{}
	tests := []struct {
		name string
		a, b domain.Location
		want domain.Metres
	}{
		{"same point", origin, origin, 0},
		{"one degree of longitude at the equator", origin, domain.Location{Lng: 1}, 111195},
		{"one degree of latitude", origin, domain.Location{Lat: 1}, 111195},
		{"antipodal", origin, domain.Location{Lng: 180}, 20015114},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.DistanceTo(tc.b); got != tc.want {
				t.Fatalf("distance = %d, want %d", got, tc.want)
			}
			if back := tc.b.DistanceTo(tc.a); back != tc.want {
				t.Fatalf("reverse distance = %d, want %d", back, tc.want)
			}
		})
	}
}
