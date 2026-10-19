// Package matching ranks candidate drivers; the booking service tries to reserve them in the returned order.
package matching

import (
	"cmp"
	"slices"

	"ridehailing/internal/domain"
)

type Nearest struct{}

func (Nearest) Rank(candidates []domain.Driver, pickup domain.Location) []domain.Driver {
	ranked := slices.Clone(candidates)
	slices.SortStableFunc(ranked, func(a, b domain.Driver) int {
		return cmp.Compare(a.Location.DistanceTo(pickup), b.Location.DistanceTo(pickup))
	})
	return ranked
}

type HighestRated struct{}

func (HighestRated) Rank(candidates []domain.Driver, pickup domain.Location) []domain.Driver {
	ranked := slices.Clone(candidates)
	slices.SortStableFunc(ranked, func(a, b domain.Driver) int {
		return cmp.Or(
			cmp.Compare(b.Rating, a.Rating),
			cmp.Compare(a.Location.DistanceTo(pickup), b.Location.DistanceTo(pickup)),
		)
	})
	return ranked
}
