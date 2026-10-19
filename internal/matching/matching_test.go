package matching_test

import (
	"testing"

	"ridehailing/internal/domain"
	"ridehailing/internal/matching"
)

var pickup = domain.Location{}

func driver(t *testing.T, name string, lng, rating float64) domain.Driver {
	t.Helper()
	d, err := domain.NewDriver(name, domain.Sedan, domain.Location{Lng: lng}, rating)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func names(drivers []domain.Driver) []string {
	out := make([]string, len(drivers))
	for i, d := range drivers {
		out[i] = d.Name
	}
	return out
}

func assertOrder(t *testing.T, got []domain.Driver, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", names(got), want)
	}
	for i := range want {
		if got[i].Name != want[i] {
			t.Fatalf("got %v, want %v", names(got), want)
		}
	}
}

func TestNearestRanksByDistance(t *testing.T) {
	in := []domain.Driver{
		driver(t, "three-km", 0.03, 5),
		driver(t, "one-km", 0.01, 3),
		driver(t, "two-km", 0.02, 4),
	}
	assertOrder(t, matching.Nearest{}.Rank(in, pickup), "one-km", "two-km", "three-km")
	if in[0].Name != "three-km" {
		t.Fatal("Rank must not reorder the caller's slice")
	}
}

func TestHighestRatedBreaksTiesByDistance(t *testing.T) {
	in := []domain.Driver{
		driver(t, "4.0-near", 0.01, 4.0),
		driver(t, "4.8-far", 0.03, 4.8),
		driver(t, "4.8-mid", 0.02, 4.8),
	}
	assertOrder(t, matching.HighestRated{}.Rank(in, pickup), "4.8-mid", "4.8-far", "4.0-near")
}

func TestRankEmpty(t *testing.T) {
	if got := (matching.Nearest{}).Rank(nil, pickup); len(got) != 0 {
		t.Fatalf("got %v, want empty", names(got))
	}
	if got := (matching.HighestRated{}).Rank(nil, pickup); len(got) != 0 {
		t.Fatalf("got %v, want empty", names(got))
	}
}
