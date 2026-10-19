package domain_test

import (
	"errors"
	"regexp"
	"testing"

	"ridehailing/internal/domain"
)

func TestParseCarType(t *testing.T) {
	tests := []struct {
		in   string
		want domain.CarType
		ok   bool
	}{
		{"HATCHBACK", domain.Hatchback, true},
		{"sedan", domain.Sedan, true},
		{" Sedan ", domain.Sedan, true},
		{"suv", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := domain.ParseCarType(tc.in)
			if !tc.ok {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("error = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestCarTypeValid(t *testing.T) {
	for _, ct := range domain.AllCarTypes() {
		if !ct.Valid() {
			t.Errorf("%q should be valid", ct)
		}
	}
	if domain.CarType("SUV").Valid() {
		t.Error("SUV should not be valid yet")
	}
}

func TestUpgradesFor(t *testing.T) {
	tests := []struct {
		in   domain.CarType
		want []domain.CarType
	}{
		{domain.Hatchback, []domain.CarType{domain.Sedan}},
		{domain.Sedan, nil},
		{"SUV", nil},
	}
	for _, tc := range tests {
		got := domain.UpgradesFor(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("UpgradesFor(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("UpgradesFor(%q) = %v, want %v", tc.in, got, tc.want)
			}
		}
	}
}

func TestPaiseINR(t *testing.T) {
	tests := []struct {
		in   domain.Paise
		want string
	}{
		{0, "0.00"},
		{5, "0.05"},
		{5400, "54.00"},
		{123456, "1234.56"},
	}
	for _, tc := range tests {
		if got := tc.in.INR(); got != tc.want {
			t.Errorf("Paise(%d).INR() = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMultiplierFloat(t *testing.T) {
	if got := domain.Multiplier(125).Float(); got != 1.25 {
		t.Errorf("Float() = %v, want 1.25", got)
	}
}

func TestParseRideStatus(t *testing.T) {
	tests := []struct {
		in   string
		want domain.RideStatus
		ok   bool
	}{
		{"ongoing", domain.RideOngoing, true},
		{"COMPLETED", domain.RideCompleted, true},
		{"Cancelled", domain.RideCancelled, true},
		{"done", "", false},
	}
	for _, tc := range tests {
		got, err := domain.ParseRideStatus(tc.in)
		if tc.ok != (err == nil) || got != tc.want {
			t.Errorf("ParseRideStatus(%q) = %q, %v; want %q, ok=%v", tc.in, got, err, tc.want, tc.ok)
		}
		if !tc.ok && !errors.Is(err, domain.ErrValidation) {
			t.Errorf("ParseRideStatus(%q) error = %v, want ErrValidation", tc.in, err)
		}
	}
}

func TestNewID(t *testing.T) {
	shape := regexp.MustCompile(`^ride_[a-z2-7]{12}$`)
	a, b := domain.NewID("ride"), domain.NewID("ride")
	if !shape.MatchString(a) {
		t.Errorf("NewID = %q, want shape %s", a, shape)
	}
	if a == b {
		t.Errorf("two IDs collided: %q", a)
	}
}
