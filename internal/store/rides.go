package store

import (
	"cmp"
	"fmt"
	"slices"
	"sync"

	"ridehailing/internal/domain"
)

type Rides struct {
	mu   sync.RWMutex
	byID map[string]domain.Ride
}

func NewRides() *Rides { return &Rides{byID: map[string]domain.Ride{}} }

func (s *Rides) Save(r domain.Ride) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byID[r.ID]; exists {
		return fmt.Errorf("%w: ride %s", domain.ErrAlreadyExists, r.ID)
	}
	s.byID[r.ID] = r.Clone()
	return nil
}

func (s *Rides) Get(id string) (domain.Ride, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.byID[id]
	if !ok {
		return domain.Ride{}, fmt.Errorf("%w: ride %s", domain.ErrNotFound, id)
	}
	return r.Clone(), nil
}

// Update runs mutate under the write lock, so a state transition is atomic: of two concurrent
// End calls on one ride, exactly one sees ONGOING. A mutation error leaves the stored ride untouched.
func (s *Rides) Update(id string, mutate func(*domain.Ride) error) (domain.Ride, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.byID[id]
	if !ok {
		return domain.Ride{}, fmt.Errorf("%w: ride %s", domain.ErrNotFound, id)
	}
	r := stored.Clone()
	if err := mutate(&r); err != nil {
		return domain.Ride{}, err
	}
	s.byID[id] = r
	return r.Clone(), nil
}

func (s *Rides) ListByUser(userID string, status *domain.RideStatus) []domain.Ride {
	return s.list(func(r domain.Ride) bool { return r.UserID == userID && matchesStatus(r, status) })
}

func (s *Rides) ListByDriver(driverID string, status *domain.RideStatus) []domain.Ride {
	return s.list(func(r domain.Ride) bool { return r.DriverID == driverID && matchesStatus(r, status) })
}

func (s *Rides) CountOngoingWithin(center domain.Location, radius domain.Metres) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, r := range s.byID {
		if r.Status == domain.RideOngoing && within(r.Pickup, center, radius) {
			n++
		}
	}
	return n
}

// list returns newest first; ties fall back to ID so the order is stable across calls.
func (s *Rides) list(keep func(domain.Ride) bool) []domain.Ride {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Ride{}
	for _, r := range s.byID {
		if keep(r) {
			out = append(out, r.Clone())
		}
	}
	slices.SortFunc(out, func(a, b domain.Ride) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), cmp.Compare(a.ID, b.ID))
	})
	return out
}

func matchesStatus(r domain.Ride, status *domain.RideStatus) bool {
	return status == nil || r.Status == *status
}
