package store

import (
	"fmt"
	"sync"

	"ridehailing/internal/domain"
)

type Drivers struct {
	mu   sync.RWMutex
	byID map[string]domain.Driver
}

func NewDrivers() *Drivers { return &Drivers{byID: map[string]domain.Driver{}} }

func (s *Drivers) Save(d domain.Driver) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byID[d.ID]; exists {
		return fmt.Errorf("%w: driver %s", domain.ErrAlreadyExists, d.ID)
	}
	s.byID[d.ID] = d
	return nil
}

func (s *Drivers) Get(id string) (domain.Driver, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.find(id)
}

func (s *Drivers) SetLocation(id string, loc domain.Location) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.find(id)
	if err != nil {
		return err
	}
	d.Location = loc
	s.byID[id] = d
	return nil
}

func (s *Drivers) AvailableWithin(center domain.Location, radius domain.Metres, carType domain.CarType) []domain.Driver {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []domain.Driver
	for _, d := range s.byID {
		if d.Status == domain.DriverAvailable && d.CarType == carType && within(d.Location, center, radius) {
			out = append(out, d)
		}
	}
	return out
}

func (s *Drivers) CountAvailableWithin(center domain.Location, radius domain.Metres) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, d := range s.byID {
		if d.Status == domain.DriverAvailable && within(d.Location, center, radius) {
			n++
		}
	}
	return n
}

// Reserve is the one place two bookings can collide: the status check and flip happen under the write lock.
func (s *Drivers) Reserve(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.find(id)
	if err != nil {
		return err
	}
	if d.Status != domain.DriverAvailable {
		return fmt.Errorf("%w: driver %s", domain.ErrDriverBusy, id)
	}
	d.Status = domain.DriverBusy
	s.byID[id] = d
	return nil
}

func (s *Drivers) Release(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.find(id)
	if err != nil {
		return err
	}
	d.Status = domain.DriverAvailable
	s.byID[id] = d
	return nil
}

// find expects the caller to hold the lock.
func (s *Drivers) find(id string) (domain.Driver, error) {
	d, ok := s.byID[id]
	if !ok {
		return domain.Driver{}, fmt.Errorf("%w: driver %s", domain.ErrNotFound, id)
	}
	return d, nil
}
