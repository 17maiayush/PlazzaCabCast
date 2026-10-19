package domain

// Fare keeps every intermediate amount so the API can show how a total was reached.
type Fare struct {
	Distance       Metres
	Metered        Paise
	MinFareApplied bool
	Base           Paise
	Surge          Multiplier
	Surged         Paise
	Discount       Paise
	Total          Paise
}

// FareRequest is built from a ride's booking-time snapshot plus the distance known at the end.
type FareRequest struct {
	CarType  CarType
	Distance Metres
	Surge    Multiplier
	Coupon   *Coupon
}

// SurgeInput is the demand/supply picture around a pickup point.
type SurgeInput struct {
	Demand int
	Supply int
}
