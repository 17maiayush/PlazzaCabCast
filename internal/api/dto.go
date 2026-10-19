package api

import (
	"fmt"
	"strings"
	"time"

	"ridehailing/internal/domain"
)

// locationDTO uses pointers so a missing coordinate is an error rather than a silent 0.
type locationDTO struct {
	Lat *float64 `json:"lat"`
	Lng *float64 `json:"lng"`
}

func (l *locationDTO) toDomain(field string) (domain.Location, error) {
	if l == nil || l.Lat == nil || l.Lng == nil {
		return domain.Location{}, fmt.Errorf("%w: %s needs lat and lng", domain.ErrValidation, field)
	}
	return domain.Location{Lat: *l.Lat, Lng: *l.Lng}, nil
}

type locationJSON struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

func toLocationJSON(l domain.Location) locationJSON { return locationJSON{Lat: l.Lat, Lng: l.Lng} }

type createUserRequest struct {
	Name string `json:"name"`
}

type userResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u domain.User) userResponse {
	return userResponse{ID: u.ID, Name: u.Name, CreatedAt: u.CreatedAt}
}

type createDriverRequest struct {
	Name     string       `json:"name"`
	CarType  string       `json:"car_type"`
	Location *locationDTO `json:"location"`
	Rating   *float64     `json:"rating"`
}

type driverResponse struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	CarType  string       `json:"car_type"`
	Location locationJSON `json:"location"`
	Rating   float64      `json:"rating"`
	Status   string       `json:"status"`
}

func toDriverResponse(d domain.Driver) driverResponse {
	return driverResponse{
		ID:       d.ID,
		Name:     d.Name,
		CarType:  string(d.CarType),
		Location: toLocationJSON(d.Location),
		Rating:   d.Rating,
		Status:   string(d.Status),
	}
}

type bookRideRequest struct {
	UserID     string       `json:"user_id"`
	CarType    string       `json:"car_type"`
	Pickup     *locationDTO `json:"pickup"`
	CouponCode string       `json:"coupon_code"`
}

type endRideRequest struct {
	Drop *locationDTO `json:"drop"`
}

type rideResponse struct {
	ID                   string        `json:"id"`
	UserID               string        `json:"user_id"`
	DriverID             string        `json:"driver_id"`
	RequestedCarType     string        `json:"requested_car_type"`
	AssignedCarType      string        `json:"assigned_car_type"`
	Upgraded             bool          `json:"upgraded"`
	Status               string        `json:"status"`
	Pickup               locationJSON  `json:"pickup"`
	Drop                 *locationJSON `json:"drop"`
	SurgeMultiplier      float64       `json:"surge_multiplier"`
	CouponCode           *string       `json:"coupon_code"`
	Fare                 *fareResponse `json:"fare"`
	CancellationFeePaise *int64        `json:"cancellation_fee_paise"`
	CreatedAt            time.Time     `json:"created_at"`
	EndedAt              *time.Time    `json:"ended_at"`
}

type fareResponse struct {
	DistanceM       int64   `json:"distance_m"`
	MeteredPaise    int64   `json:"metered_paise"`
	MinFareApplied  bool    `json:"min_fare_applied"`
	BasePaise       int64   `json:"base_paise"`
	SurgeMultiplier float64 `json:"surge_multiplier"`
	SurgedPaise     int64   `json:"surged_paise"`
	DiscountPaise   int64   `json:"discount_paise"`
	TotalPaise      int64   `json:"total_paise"`
	TotalINR        string  `json:"total_inr"`
}

func toRideResponse(r domain.Ride) rideResponse {
	out := rideResponse{
		ID:               r.ID,
		UserID:           r.UserID,
		DriverID:         r.DriverID,
		RequestedCarType: string(r.RequestedCarType),
		AssignedCarType:  string(r.AssignedCarType),
		Upgraded:         r.Upgraded,
		Status:           string(r.Status),
		Pickup:           toLocationJSON(r.Pickup),
		SurgeMultiplier:  r.Surge.Float(),
		CreatedAt:        r.CreatedAt,
		EndedAt:          r.EndedAt,
	}
	if r.Drop != nil {
		drop := toLocationJSON(*r.Drop)
		out.Drop = &drop
	}
	if r.Coupon != nil {
		code := r.Coupon.Code
		out.CouponCode = &code
	}
	if r.Fare != nil {
		fare := toFareResponse(*r.Fare)
		out.Fare = &fare
	}
	if r.Status == domain.RideCancelled {
		fee := int64(r.CancellationFee)
		out.CancellationFeePaise = &fee
	}
	return out
}

func toFareResponse(f domain.Fare) fareResponse {
	return fareResponse{
		DistanceM:       int64(f.Distance),
		MeteredPaise:    int64(f.Metered),
		MinFareApplied:  f.MinFareApplied,
		BasePaise:       int64(f.Base),
		SurgeMultiplier: f.Surge.Float(),
		SurgedPaise:     int64(f.Surged),
		DiscountPaise:   int64(f.Discount),
		TotalPaise:      int64(f.Total),
		TotalINR:        f.Total.INR(),
	}
}

type ridesResponse struct {
	Rides []rideResponse `json:"rides"`
}

func toRidesResponse(rides []domain.Ride) ridesResponse {
	out := ridesResponse{Rides: make([]rideResponse, len(rides))}
	for i, r := range rides {
		out.Rides[i] = toRideResponse(r)
	}
	return out
}

type createCouponRequest struct {
	Code        string     `json:"code"`
	Kind        string     `json:"kind"`
	Percent     *int       `json:"percent"`
	AmountPaise *int64     `json:"amount_paise"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

func (req createCouponRequest) toDomain() (domain.Coupon, error) {
	switch domain.CouponKind(strings.ToUpper(req.Kind)) {
	case domain.CouponPercent:
		if req.Percent == nil || req.AmountPaise != nil {
			return domain.Coupon{}, fmt.Errorf("%w: a PERCENT coupon takes percent and nothing else", domain.ErrValidation)
		}
		return domain.NewPercentCoupon(req.Code, *req.Percent, req.ExpiresAt)
	case domain.CouponFlat:
		if req.AmountPaise == nil || req.Percent != nil {
			return domain.Coupon{}, fmt.Errorf("%w: a FLAT coupon takes amount_paise and nothing else", domain.ErrValidation)
		}
		return domain.NewFlatCoupon(req.Code, domain.Paise(*req.AmountPaise), req.ExpiresAt)
	}
	return domain.Coupon{}, fmt.Errorf("%w: kind must be PERCENT or FLAT", domain.ErrValidation)
}

type couponResponse struct {
	Code        string     `json:"code"`
	Kind        string     `json:"kind"`
	Percent     *int       `json:"percent"`
	AmountPaise *int64     `json:"amount_paise"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

func toCouponResponse(c domain.Coupon) couponResponse {
	out := couponResponse{Code: c.Code, Kind: string(c.Kind), ExpiresAt: c.ExpiresAt}
	switch c.Kind {
	case domain.CouponPercent:
		out.Percent = &c.Percent
	case domain.CouponFlat:
		amount := int64(c.Amount)
		out.AmountPaise = &amount
	}
	return out
}
