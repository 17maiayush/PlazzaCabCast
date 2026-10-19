package api_test

import (
	"fmt"
	"net/http"
	"testing"
)

type estimateJSON struct {
	RequestedCarType string    `json:"requested_car_type"`
	AssignedCarType  *string   `json:"assigned_car_type"`
	DriverAvailable  bool      `json:"driver_available"`
	Upgraded         bool      `json:"upgraded"`
	CouponCode       *string   `json:"coupon_code"`
	Fare             *fareJSON `json:"fare"`
}

func estimateBody(carType, coupon string) string {
	return fmt.Sprintf(`{"car_type":%q,"pickup":{"lat":0,"lng":0},"drop":{"lat":0,"lng":0.062953},"coupon_code":%q}`, carType, coupon)
}

func TestEstimate(t *testing.T) {
	h, _ := newAPI(t)

	none := decode[estimateJSON](t, do(t, h, "POST", "/rides/estimate", estimateBody("hatchback", "")), http.StatusOK)
	if none.DriverAvailable || none.AssignedCarType != nil || none.Upgraded || none.Fare.TotalPaise != 5400 || none.CouponCode != nil {
		t.Fatalf("estimate with no drivers = %+v", none)
	}

	createDriver(t, h, "sedan", 0.01)
	decode[couponJSON](t, do(t, h, "POST", "/coupons", `{"code":"save20","kind":"percent","percent":20}`), http.StatusCreated)
	up := decode[estimateJSON](t, do(t, h, "POST", "/rides/estimate", estimateBody("hatchback", "save20")), http.StatusOK)
	if !up.DriverAvailable || up.AssignedCarType == nil || *up.AssignedCarType != "SEDAN" || !up.Upgraded {
		t.Fatalf("estimate = %+v, want a sedan upgrade", up)
	}
	if up.Fare.TotalPaise != 4320 || up.CouponCode == nil || *up.CouponCode != "SAVE20" || up.RequestedCarType != "HATCHBACK" {
		t.Fatalf("estimate = %+v, want 20%% off the hatchback price", up)
	}

	for _, body := range []string{
		`{"car_type":"hatchback","pickup":{"lat":0,"lng":0}}`,
		`{"car_type":"hatchback","drop":{"lat":0,"lng":0}}`,
		`{"car_type":"suv","pickup":{"lat":0,"lng":0},"drop":{"lat":0,"lng":0}}`,
		`{"car_type":`,
	} {
		assertError(t, do(t, h, "POST", "/rides/estimate", body), http.StatusBadRequest, "VALIDATION_ERROR")
	}
	assertError(t, do(t, h, "POST", "/rides/estimate", estimateBody("hatchback", "nope")), http.StatusBadRequest, "INVALID_COUPON")
}

func TestListEndpoints(t *testing.T) {
	h, _ := newAPI(t)
	empty := decode[map[string][]any](t, do(t, h, "GET", "/users", ""), http.StatusOK)
	if users, ok := empty["users"]; !ok || len(users) != 0 {
		t.Fatalf("empty users list = %v, want an empty array, not null", empty)
	}

	createUser(t, h)
	driverID := createDriver(t, h, "sedan", 0.01)
	decode[couponJSON](t, do(t, h, "POST", "/coupons", `{"code":"flat30","kind":"flat","amount_paise":3000}`), http.StatusCreated)

	users := decode[map[string][]map[string]any](t, do(t, h, "GET", "/users", ""), http.StatusOK)
	if len(users["users"]) != 1 || users["users"][0]["name"] != "Asha" {
		t.Fatalf("users = %v", users)
	}
	drivers := decode[map[string][]driverJSON](t, do(t, h, "GET", "/drivers", ""), http.StatusOK)
	if len(drivers["drivers"]) != 1 || drivers["drivers"][0].ID != driverID || drivers["drivers"][0].Status != "AVAILABLE" {
		t.Fatalf("drivers = %+v", drivers)
	}
	coupons := decode[map[string][]couponJSON](t, do(t, h, "GET", "/coupons", ""), http.StatusOK)
	if len(coupons["coupons"]) != 1 || coupons["coupons"][0].Code != "FLAT30" {
		t.Fatalf("coupons = %+v", coupons)
	}
}
