// Package store holds the in-memory repositories. Each one guards its map with its own mutex,
// never takes another repository's lock, and hands out copies rather than references.
package store

import (
	"fmt"
	"sync"

	"ridehailing/internal/domain"
)

type Users struct {
	mu   sync.RWMutex
	byID map[string]domain.User
}

func NewUsers() *Users { return &Users{byID: map[string]domain.User{}} }

func (s *Users) Save(u domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byID[u.ID]; exists {
		return fmt.Errorf("%w: user %s", domain.ErrAlreadyExists, u.ID)
	}
	s.byID[u.ID] = u
	return nil
}

func (s *Users) Get(id string) (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.byID[id]
	if !ok {
		return domain.User{}, fmt.Errorf("%w: user %s", domain.ErrNotFound, id)
	}
	return u, nil
}

type Coupons struct {
	mu     sync.RWMutex
	byCode map[string]domain.Coupon
}

func NewCoupons() *Coupons { return &Coupons{byCode: map[string]domain.Coupon{}} }

func (s *Coupons) Save(c domain.Coupon) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byCode[c.Code]; exists {
		return fmt.Errorf("%w: coupon %s", domain.ErrAlreadyExists, c.Code)
	}
	s.byCode[c.Code] = c.Clone()
	return nil
}

func (s *Coupons) Get(code string) (domain.Coupon, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.byCode[code]
	if !ok {
		return domain.Coupon{}, fmt.Errorf("%w: coupon %s", domain.ErrNotFound, code)
	}
	return c.Clone(), nil
}

func (s *Coupons) Delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byCode[code]; !ok {
		return fmt.Errorf("%w: coupon %s", domain.ErrNotFound, code)
	}
	delete(s.byCode, code)
	return nil
}

func within(loc, center domain.Location, radius domain.Metres) bool {
	return loc.DistanceTo(center) <= radius
}
