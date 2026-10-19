// Package api is the HTTP adapter: routing, wire formats, request validation and the one table
// that maps domain errors to status codes. It knows nothing about how rides are matched or priced.
package api

import (
	"log/slog"
	"net/http"

	"ridehailing/internal/domain"
	"ridehailing/internal/service"
)

type Services struct {
	Users   *service.Users
	Drivers *service.Drivers
	Rides   *service.Rides
	Coupons *service.Coupons
}

type IdempotencyStore interface {
	Begin(scope, fingerprint string) (*domain.IdempotentResponse, error)
	Commit(scope string, resp domain.IdempotentResponse)
	Abort(scope string)
}

type handler struct {
	svc    Services
	logger *slog.Logger
}

func NewHandler(svc Services, idem IdempotencyStore, logger *slog.Logger) http.Handler {
	h := &handler{svc: svc, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.handle(h.healthz))
	mux.HandleFunc("GET /users", h.handle(h.listUsers))
	mux.HandleFunc("POST /users", h.handle(h.createUser))
	mux.HandleFunc("GET /drivers", h.handle(h.listDrivers))
	mux.HandleFunc("POST /drivers", h.handle(h.createDriver))
	mux.HandleFunc("PUT /drivers/{id}/location", h.handle(h.updateDriverLocation))
	mux.HandleFunc("POST /rides/estimate", h.handle(h.estimateRide))
	mux.Handle("POST /rides", h.idempotent(idem, h.handle(h.bookRide)))
	mux.HandleFunc("POST /rides/{id}/end", h.handle(h.endRide))
	mux.HandleFunc("POST /rides/{id}/cancel", h.handle(h.cancelRide))
	mux.HandleFunc("GET /users/{id}/rides", h.handle(h.userRides))
	mux.HandleFunc("GET /drivers/{id}/rides", h.handle(h.driverRides))
	mux.HandleFunc("GET /coupons", h.handle(h.listCoupons))
	mux.HandleFunc("POST /coupons", h.handle(h.createCoupon))
	mux.HandleFunc("DELETE /coupons/{code}", h.handle(h.deleteCoupon))
	return limitBody(mux)
}

const maxBodyBytes = 1 << 20

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}
