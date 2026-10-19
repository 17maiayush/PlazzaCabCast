package service_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"ridehailing/internal/domain"
	"ridehailing/internal/service"
	"ridehailing/internal/store"
)

// flakyDrivers is the real store with injectable failures for the steps that run after a commit.
type flakyDrivers struct {
	*store.Drivers
	reserveErr, releaseErr, setLocationErr error
}

func (f *flakyDrivers) Reserve(id string) error {
	if f.reserveErr != nil {
		return f.reserveErr
	}
	return f.Drivers.Reserve(id)
}

func (f *flakyDrivers) Release(id string) error {
	if f.releaseErr != nil {
		return f.releaseErr
	}
	return f.Drivers.Release(id)
}

func (f *flakyDrivers) SetLocation(id string, loc domain.Location) error {
	if f.setLocationErr != nil {
		return f.setLocationErr
	}
	return f.Drivers.SetLocation(id, loc)
}

type failingRides struct {
	*store.Rides
	saveErr error
	panics  bool
}

func (f *failingRides) Save(r domain.Ride) error {
	if f.panics {
		panic("store exploded")
	}
	if f.saveErr != nil {
		return f.saveErr
	}
	return f.Rides.Save(r)
}

func TestBookReleasesDriverWhenSaveFails(t *testing.T) {
	boom := errors.New("disk full")
	h := newHarness(t, func(d *service.RidesDeps) { d.Rides = &failingRides{Rides: d.Rides.(*store.Rides), saveErr: boom} })
	u := h.user(t)
	d := h.driver(t, domain.Hatchback, 0.01, 4)

	if _, err := h.rides.Book(service.BookRequest{UserID: u.ID, CarType: domain.Hatchback, Pickup: pickup}); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the save failure", err)
	}
	if h.driverStatus(t, d.ID) != domain.DriverAvailable {
		t.Fatal("a failed save must release the reserved driver")
	}
}

func TestBookReleasesDriverWhenSavePanics(t *testing.T) {
	h := newHarness(t, func(d *service.RidesDeps) { d.Rides = &failingRides{Rides: d.Rides.(*store.Rides), panics: true} })
	u := h.user(t)
	d := h.driver(t, domain.Hatchback, 0.01, 4)

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected the panic to propagate")
			}
		}()
		_, _ = h.rides.Book(service.BookRequest{UserID: u.ID, CarType: domain.Hatchback, Pickup: pickup})
	}()
	if h.driverStatus(t, d.ID) != domain.DriverAvailable {
		t.Fatal("a panic after reserve must still release the driver")
	}
}

func TestBookFailsWhenEveryCandidateIsTakenMeanwhile(t *testing.T) {
	h := newHarness(t, func(d *service.RidesDeps) {
		d.Drivers = &flakyDrivers{Drivers: d.Drivers.(*store.Drivers), reserveErr: domain.ErrDriverBusy}
	})
	u := h.user(t)
	h.driver(t, domain.Hatchback, 0.01, 4)
	if _, err := h.rides.Book(service.BookRequest{UserID: u.ID, CarType: domain.Hatchback, Pickup: pickup}); !errors.Is(err, domain.ErrNoDriverAvailable) {
		t.Fatalf("error = %v, want ErrNoDriverAvailable", err)
	}
}

func TestEndAndCancelSurfaceDriverReleaseFailures(t *testing.T) {
	boom := errors.New("driver store down")
	tests := []struct {
		name  string
		tweak func(*flakyDrivers)
		act   func(h *harness, rideID string) (domain.Ride, error)
	}{
		{"end: release fails", func(f *flakyDrivers) { f.releaseErr = boom }, func(h *harness, id string) (domain.Ride, error) { return h.rides.End(id, oneKm) }},
		{"end: relocation fails", func(f *flakyDrivers) { f.setLocationErr = boom }, func(h *harness, id string) (domain.Ride, error) { return h.rides.End(id, oneKm) }},
		{"cancel: release fails", func(f *flakyDrivers) { f.releaseErr = boom }, func(h *harness, id string) (domain.Ride, error) { return h.rides.Cancel(id) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var flaky *flakyDrivers
			h := newHarness(t, func(d *service.RidesDeps) {
				flaky = &flakyDrivers{Drivers: d.Drivers.(*store.Drivers)}
				d.Drivers = flaky
			})
			u := h.user(t)
			h.driver(t, domain.Hatchback, 0.01, 4)
			r := h.book(t, u.ID, "")
			tc.tweak(flaky)

			if _, err := tc.act(h, r.ID); !errors.Is(err, boom) {
				t.Fatalf("error = %v, want the driver failure", err)
			}
			if stored, _ := h.rides.ForUser(u.ID, nil); stored[0].Status == domain.RideOngoing {
				t.Fatal("the ride transition was saved before the driver step, so it must stay saved")
			}
		})
	}
}

func TestConcurrentBookingsForOneDriver(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	h.driver(t, domain.Hatchback, 0.01, 4)

	var wg sync.WaitGroup
	var booked, refused atomic.Int32
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.rides.Book(service.BookRequest{UserID: u.ID, CarType: domain.Hatchback, Pickup: pickup})
			switch {
			case err == nil:
				booked.Add(1)
			case errors.Is(err, domain.ErrNoDriverAvailable):
				refused.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if booked.Load() != 1 || refused.Load() != 49 {
		t.Fatalf("booked=%d refused=%d, want exactly 1 and 49", booked.Load(), refused.Load())
	}
}

func TestConcurrentBookingsNeverShareADriver(t *testing.T) {
	h := newHarness(t)
	u := h.user(t)
	const drivers = 10
	for i := range drivers {
		h.driver(t, domain.Hatchback, 0.001*float64(i+1), 4)
	}

	var mu sync.Mutex
	assigned := map[string]int{}
	var wg sync.WaitGroup
	for range 2 * drivers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := h.rides.Book(service.BookRequest{UserID: u.ID, CarType: domain.Hatchback, Pickup: pickup})
			if err != nil {
				return
			}
			mu.Lock()
			assigned[r.DriverID]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(assigned) != drivers {
		t.Fatalf("%d drivers assigned, want %d", len(assigned), drivers)
	}
	for id, n := range assigned {
		if n != 1 {
			t.Fatalf("driver %s was booked %d times", id, n)
		}
	}
}

type failingFares struct{ err error }

func (f failingFares) Calculate(domain.FareRequest) (domain.Fare, error) { return domain.Fare{}, f.err }

func TestEndSurfacesFareErrors(t *testing.T) {
	boom := errors.New("tariff missing")
	h := newHarness(t, func(d *service.RidesDeps) { d.Fares = failingFares{err: boom} })
	u := h.user(t)
	d := h.driver(t, domain.Hatchback, 0.01, 4)
	r := h.book(t, u.ID, "")

	if _, err := h.rides.End(r.ID, oneKm); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the fare failure", err)
	}
	if h.driverStatus(t, d.ID) != domain.DriverBusy {
		t.Fatal("a fare failure must leave the ride ongoing and the driver reserved")
	}
}
