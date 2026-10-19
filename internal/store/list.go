package store

import (
	"cmp"
	"maps"
	"slices"

	"ridehailing/internal/domain"
)

func (s *Users) List() []domain.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.SortedFunc(maps.Values(s.byID), func(a, b domain.User) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
}

func (s *Drivers) List() []domain.Driver {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.SortedFunc(maps.Values(s.byID), func(a, b domain.Driver) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
}

func (s *Coupons) List() []domain.Coupon {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Coupon, 0, len(s.byCode))
	for _, c := range s.byCode {
		out = append(out, c.Clone())
	}
	slices.SortFunc(out, func(a, b domain.Coupon) int { return cmp.Compare(a.Code, b.Code) })
	return out
}
