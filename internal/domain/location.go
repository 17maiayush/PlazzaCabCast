package domain

import (
	"fmt"
	"math"
)

type Location struct {
	Lat float64
	Lng float64
}

func (l Location) Validate() error {
	if math.IsNaN(l.Lat) || l.Lat < -90 || l.Lat > 90 {
		return fmt.Errorf("%w: latitude %v out of range", ErrValidation, l.Lat)
	}
	if math.IsNaN(l.Lng) || l.Lng < -180 || l.Lng > 180 {
		return fmt.Errorf("%w: longitude %v out of range", ErrValidation, l.Lng)
	}
	return nil
}

// Mean Earth radius. Spherical haversine is within 0.5% of the true distance, plenty for matching and fares.
const earthRadiusM = 6371008.8

// DistanceTo is the great-circle distance, rounded once so everything downstream stays integer.
func (l Location) DistanceTo(o Location) Metres {
	lat1, lat2 := radians(l.Lat), radians(o.Lat)
	sinLat, sinLng := math.Sin(radians(o.Lat-l.Lat)/2), math.Sin(radians(o.Lng-l.Lng)/2)
	a := sinLat*sinLat + math.Cos(lat1)*math.Cos(lat2)*sinLng*sinLng
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return Metres(math.Round(earthRadiusM * c))
}

func radians(deg float64) float64 { return deg * math.Pi / 180 }
