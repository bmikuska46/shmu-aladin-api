package geo

import "math"

const earthRadiusKm = 6371.0

// HaversineKm returns the great-circle distance in kilometres between two WGS84 points.
func HaversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
}

// RoundKm rounds a distance to one decimal place (0.1 km).
func RoundKm(km float64) float64 {
	return math.Round(km*10) / 10
}
