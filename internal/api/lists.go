package api

import (
	"net/http"

	"ridehailing/internal/domain"
	"ridehailing/internal/service"
)

func (h *handler) listUsers(*http.Request) (int, any, error) {
	return http.StatusOK, map[string][]userResponse{"users": mapSlice(h.svc.Users.List(), toUserResponse)}, nil
}

func (h *handler) listDrivers(*http.Request) (int, any, error) {
	return http.StatusOK, map[string][]driverResponse{"drivers": mapSlice(h.svc.Drivers.List(), toDriverResponse)}, nil
}

func (h *handler) listCoupons(*http.Request) (int, any, error) {
	return http.StatusOK, map[string][]couponResponse{"coupons": mapSlice(h.svc.Coupons.List(), toCouponResponse)}, nil
}

// mapSlice always returns a non-nil slice so an empty list encodes as [] rather than null.
func mapSlice[T, R any](in []T, fn func(T) R) []R {
	out := make([]R, len(in))
	for i, v := range in {
		out[i] = fn(v)
	}
	return out
}

type estimateRideRequest struct {
	CarType    string       `json:"car_type"`
	Pickup     *locationDTO `json:"pickup"`
	Drop       *locationDTO `json:"drop"`
	CouponCode string       `json:"coupon_code"`
}

type estimateResponse struct {
	RequestedCarType string       `json:"requested_car_type"`
	AssignedCarType  *string      `json:"assigned_car_type"`
	DriverAvailable  bool         `json:"driver_available"`
	Upgraded         bool         `json:"upgraded"`
	CouponCode       *string      `json:"coupon_code"`
	Fare             fareResponse `json:"fare"`
}

func (h *handler) estimateRide(r *http.Request) (int, any, error) {
	var req estimateRideRequest
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
	drop, err := req.Drop.toDomain("drop")
	if err != nil {
		return 0, nil, err
	}
	est, err := h.svc.Rides.Estimate(service.EstimateRequest{CarType: carType, Pickup: pickup, Drop: drop, CouponCode: req.CouponCode})
	if err != nil {
		return 0, nil, err
	}

	out := estimateResponse{
		RequestedCarType: string(carType),
		DriverAvailable:  est.AssignedCarType != "",
		Upgraded:         est.AssignedCarType != "" && est.AssignedCarType != carType,
		Fare:             toFareResponse(est.Fare),
	}
	if est.AssignedCarType != "" {
		assigned := string(est.AssignedCarType)
		out.AssignedCarType = &assigned
	}
	if est.Coupon != nil {
		out.CouponCode = &est.Coupon.Code
	}
	return http.StatusOK, out, nil
}
