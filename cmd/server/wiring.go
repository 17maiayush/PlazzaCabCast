package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"ridehailing/internal/api"
	"ridehailing/internal/cancellation"
	"ridehailing/internal/matching"
	"ridehailing/internal/pricing"
	"ridehailing/internal/service"
	"ridehailing/internal/store"
	"ridehailing/internal/web"
)

// newHandler is the only place concrete adapters meet the service. Swapping a strategy is a
// one-line change here; the booking logic never sees the difference.
func newHandler(cfg config, out io.Writer) (http.Handler, error) {
	calc, err := pricing.NewCalculator(cfg.Tariffs)
	if err != nil {
		return nil, err
	}
	match, err := matchStrategy(cfg.Matching)
	if err != nil {
		return nil, err
	}
	surge, err := surgeStrategy(cfg.Surge)
	if err != nil {
		return nil, err
	}

	users, drivers, rides, coupons := store.NewUsers(), store.NewDrivers(), store.NewRides(), store.NewCoupons()
	svc := api.Services{
		Users:   service.NewUsers(users, time.Now),
		Drivers: service.NewDrivers(drivers),
		Coupons: service.NewCoupons(coupons),
		Rides: service.NewRides(service.RidesDeps{
			Users:        users,
			Drivers:      drivers,
			Rides:        rides,
			Coupons:      coupons,
			Fares:        calc,
			Match:        match,
			Surge:        surge,
			Cancellation: cancellation.GracePolicy{Grace: cfg.CancelGrace, Amount: cfg.CancelFee},
			Radius:       cfg.Radius,
			Now:          time.Now,
		}),
	}
	logger := slog.New(slog.NewTextHandler(out, nil))

	root := http.NewServeMux()
	root.Handle("GET /{$}", web.Handler())
	root.Handle("/", api.NewHandler(svc, store.NewIdempotency(), logger))
	return root, nil
}

func matchStrategy(name string) (service.MatchStrategy, error) {
	switch name {
	case "nearest":
		return matching.Nearest{}, nil
	case "rated":
		return matching.HighestRated{}, nil
	}
	return nil, fmt.Errorf("MATCHING must be nearest or rated, got %q", name)
}

func surgeStrategy(name string) (service.SurgeStrategy, error) {
	switch name {
	case "demand":
		return pricing.DefaultStepSurge(), nil
	case "off":
		return pricing.NoSurge{}, nil
	}
	return nil, fmt.Errorf("SURGE must be demand or off, got %q", name)
}
