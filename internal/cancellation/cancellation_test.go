package cancellation_test

import (
	"testing"
	"time"

	"ridehailing/internal/cancellation"
	"ridehailing/internal/domain"
)

func TestGracePolicy(t *testing.T) {
	booked := time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)
	ride := domain.Ride{CreatedAt: booked}
	policy := cancellation.GracePolicy{Grace: 3 * time.Minute, Amount: 2000}

	tests := []struct {
		name string
		at   time.Time
		want domain.Paise
	}{
		{"immediately", booked, 0},
		{"inside the window", booked.Add(2 * time.Minute), 0},
		{"at the boundary", booked.Add(3 * time.Minute), 0},
		{"just after the window", booked.Add(3*time.Minute + time.Second), 2000},
		{"much later", booked.Add(time.Hour), 2000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := policy.Fee(ride, tc.at); got != tc.want {
				t.Fatalf("Fee = %d, want %d", got, tc.want)
			}
		})
	}
}
