package main

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"ridehailing/internal/domain"
	"ridehailing/internal/pricing"
)

type config struct {
	Port        string
	Radius      domain.Metres
	Matching    string // nearest | rated
	Surge       string // demand | off
	CancelGrace time.Duration
	CancelFee   domain.Paise
	Tariffs     map[domain.CarType]pricing.Tariff
}

// loadConfig parses numbers and ranges; strategy names are resolved in newHandler, where the
// implementations live.
func loadConfig(getenv func(string) string) (config, error) {
	cfg := config{
		Port:        "8080",
		Radius:      5000,
		Matching:    "nearest",
		Surge:       "demand",
		CancelGrace: 3 * time.Minute,
		CancelFee:   2000,
		Tariffs:     pricing.DefaultTariffs(),
	}
	if v := getenv("MATCHING"); v != "" {
		cfg.Matching = v
	}
	if v := getenv("SURGE"); v != "" {
		cfg.Surge = v
	}
	if v := getenv("MATCH_RADIUS_KM"); v != "" {
		km, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(km) || math.IsInf(km, 0) || km <= 0 {
			return config{}, fmt.Errorf("MATCH_RADIUS_KM must be a positive number, got %q", v)
		}
		cfg.Radius = domain.Metres(math.Round(km * 1000))
	}

	port, ok, err := intVar(getenv, "PORT", 0, 65535)
	if err != nil {
		return config{}, err
	}
	if ok {
		cfg.Port = strconv.Itoa(port)
	}
	grace, ok, err := intVar(getenv, "CANCEL_GRACE_MIN", 0, math.MaxInt32)
	if err != nil {
		return config{}, err
	}
	if ok {
		cfg.CancelGrace = time.Duration(grace) * time.Minute
	}
	fee, ok, err := intVar(getenv, "CANCEL_FEE_INR", 0, math.MaxInt32)
	if err != nil {
		return config{}, err
	}
	if ok {
		cfg.CancelFee = domain.Paise(fee) * 100
	}
	return cfg, nil
}

func intVar(getenv func(string) string, key string, lo, hi int) (value int, set bool, err error) {
	raw := getenv(key)
	if raw == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		return 0, false, fmt.Errorf("%s must be a whole number between %d and %d, got %q", key, lo, hi, raw)
	}
	return n, true, nil
}
