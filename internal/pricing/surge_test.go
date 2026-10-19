package pricing_test

import (
	"testing"

	"ridehailing/internal/domain"
	"ridehailing/internal/pricing"
)

func TestNoSurge(t *testing.T) {
	if got := (pricing.NoSurge{}).Multiplier(domain.SurgeInput{Demand: 50, Supply: 0}); got != 100 {
		t.Fatalf("NoSurge = %d, want 100", got)
	}
}

func TestStepSurge(t *testing.T) {
	tests := []struct {
		demand, supply int
		want           domain.Multiplier
	}{
		{0, 0, 100},
		{0, 5, 100},
		{1, 1, 100},
		{2, 1, 125},
		{3, 1, 150},
		{4, 1, 200},
		{1, 0, 200},
		{5, 2, 150},
		{7, 2, 200},
	}
	surge := pricing.DefaultStepSurge()
	for _, tc := range tests {
		if got := surge.Multiplier(domain.SurgeInput{Demand: tc.demand, Supply: tc.supply}); got != tc.want {
			t.Errorf("demand %d, supply %d: got %d, want %d", tc.demand, tc.supply, got, tc.want)
		}
	}
}
