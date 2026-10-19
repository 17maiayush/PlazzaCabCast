package api

import (
	"net/http"

	"ridehailing/internal/domain"
	"ridehailing/internal/service"
)

func (h *handler) healthz(*http.Request) (int, any, error) {
	return http.StatusOK, map[string]string{"status": "ok"}, nil
}

func (h *handler) createUser(r *http.Request) (int, any, error) {
	var req createUserRequest
	if err := decodeJSON(r, &req); err != nil {
		return 0, nil, err
	}
	u, err := h.svc.Users.Register(req.Name)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, toUserResponse(u), nil
}

func (h *handler) createDriver(r *http.Request) (int, any, error) {
	var req createDriverRequest
	if err := decodeJSON(r, &req); err != nil {
		return 0, nil, err
	}
	carType, err := domain.ParseCarType(req.CarType)
	if err != nil {
		return 0, nil, err
	}
	loc, err := req.Location.toDomain("location")
	if err != nil {
		return 0, nil, err
	}
	rating := 5.0 // a new driver starts with a clean slate
	if req.Rating != nil {
		rating = *req.Rating
	}
	d, err := h.svc.Drivers.Register(req.Name, carType, loc, rating)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, toDriverResponse(d), nil
}

func (h *handler) updateDriverLocation(r *http.Request) (int, any, error) {
	var req locationDTO
	if err := decodeJSON(r, &req); err != nil {
		return 0, nil, err
	}
	loc, err := req.toDomain("body")
	if err != nil {
		return 0, nil, err
	}
	d, err := h.svc.Drivers.UpdateLocation(r.PathValue("id"), loc)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, toDriverResponse(d), nil
}

func (h *handler) bookRide(r *http.Request) (int, any, error) {
	var req bookRideRequest
	if err := decodeJSON(r, &req); err != nil {
		return 0, nil, err
	}
	carType, err := domain.ParseCarType(req.CarType)
	if err != nil {
		return 0, nil, err
	}
	pickup, err := req.Pickup.toDomain("pickup")
	if err != nil {
		return 0, nil, err
	}
	ride, err := h.svc.Rides.Book(service.BookRequest{
		UserID:     req.UserID,
		CarType:    carType,
		Pickup:     pickup,
		CouponCode: req.CouponCode,
	})
	if err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, toRideResponse(ride), nil
}

func (h *handler) endRide(r *http.Request) (int, any, error) {
	var req endRideRequest
	if err := decodeJSON(r, &req); err != nil {
		return 0, nil, err
	}
	drop, err := req.Drop.toDomain("drop")
	if err != nil {
		return 0, nil, err
	}
	ride, err := h.svc.Rides.End(r.PathValue("id"), drop)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, toRideResponse(ride), nil
}

func (h *handler) cancelRide(r *http.Request) (int, any, error) {
	ride, err := h.svc.Rides.Cancel(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, toRideResponse(ride), nil
}

func (h *handler) userRides(r *http.Request) (int, any, error) {
	status, err := statusFilter(r)
	if err != nil {
		return 0, nil, err
	}
	rides, err := h.svc.Rides.ForUser(r.PathValue("id"), status)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, toRidesResponse(rides), nil
}

func (h *handler) driverRides(r *http.Request) (int, any, error) {
	status, err := statusFilter(r)
	if err != nil {
		return 0, nil, err
	}
	rides, err := h.svc.Rides.ForDriver(r.PathValue("id"), status)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, toRidesResponse(rides), nil
}

func statusFilter(r *http.Request) (*domain.RideStatus, error) {
	raw := r.URL.Query().Get("status")
	if raw == "" {
		return nil, nil
	}
	status, err := domain.ParseRideStatus(raw)
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func (h *handler) createCoupon(r *http.Request) (int, any, error) {
	var req createCouponRequest
	if err := decodeJSON(r, &req); err != nil {
		return 0, nil, err
	}
	c, err := req.toDomain()
	if err != nil {
		return 0, nil, err
	}
	if err := h.svc.Coupons.Add(c); err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, toCouponResponse(c), nil
}

func (h *handler) deleteCoupon(r *http.Request) (int, any, error) {
	if err := h.svc.Coupons.Delete(r.PathValue("code")); err != nil {
		return 0, nil, err
	}
	return http.StatusNoContent, nil, nil
}
