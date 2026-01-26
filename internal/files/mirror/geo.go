// Package mirror implements mirror groups and geographic routing for file distribution.
package mirror

import (
	"math"
	"sort"
	"strings"
)

const (
	earthRadiusKm = 6371.0
)

// GeoRouter routes requests based on geographic location.
type GeoRouter struct {
	// Fallback router when geo routing fails
	fallback ReadRouter

	// OverrideRules allows forcing specific paths to specific mirrors
	overrideRules []OverrideRule
}

// OverrideRule forces specific paths to specific mirrors.
type OverrideRule struct {
	// PathPrefix matches paths starting with this prefix.
	PathPrefix string `json:"path_prefix" yaml:"path_prefix"`

	// Namespace matches this namespace.
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`

	// MirrorIDs are the allowed mirrors for this rule.
	MirrorIDs []string `json:"mirror_ids" yaml:"mirror_ids"`
}

// NewGeoRouter creates a new geographic router.
func NewGeoRouter(fallback ReadRouter) *GeoRouter {
	if fallback == nil {
		fallback = NewFailoverRouter()
	}
	return &GeoRouter{
		fallback: fallback,
	}
}

// AddOverride adds an override rule.
func (r *GeoRouter) AddOverride(rule OverrideRule) {
	r.overrideRules = append(r.overrideRules, rule)
}

// SelectForRead returns mirrors sorted by geographic distance.
func (r *GeoRouter) SelectForRead(group *MirrorGroup, agentLocation *Location) []*Mirror {
	mirrors := group.GetHealthyMirrors()
	if len(mirrors) == 0 {
		return nil
	}

	// If no agent location, use fallback router
	if agentLocation == nil {
		return r.fallback.SelectForRead(group, agentLocation)
	}

	// Calculate distances and sort
	type mirrorDistance struct {
		mirror   *Mirror
		distance float64
	}

	distances := make([]mirrorDistance, 0, len(mirrors))
	for _, m := range mirrors {
		dist := r.calculateDistance(agentLocation, m.Location)
		distances = append(distances, mirrorDistance{mirror: m, distance: dist})
	}

	// Sort by distance
	sort.Slice(distances, func(i, j int) bool {
		return distances[i].distance < distances[j].distance
	})

	result := make([]*Mirror, len(distances))
	for i, d := range distances {
		result[i] = d.mirror
	}

	return result
}

// calculateDistance calculates distance between two locations.
func (r *GeoRouter) calculateDistance(a, b *Location) float64 {
	if b == nil {
		return math.MaxFloat64
	}

	// First, try coordinate-based distance
	if a.Latitude != 0 && a.Longitude != 0 && b.Latitude != 0 && b.Longitude != 0 {
		return haversineDistance(a.Latitude, a.Longitude, b.Latitude, b.Longitude)
	}

	// Fallback to region/zone matching
	score := 0.0

	// Same datacenter is closest
	if a.Datacenter != "" && a.Datacenter == b.Datacenter {
		return score
	}
	score += 100

	// Same zone is next closest
	if a.Zone != "" && a.Zone == b.Zone {
		return score
	}
	score += 1000

	// Same region is next
	if a.Region != "" && a.Region == b.Region {
		return score
	}
	score += 10000

	// Same country
	if a.Country != "" && a.Country == b.Country {
		return score
	}
	score += 100000

	return score
}

// haversineDistance calculates the great-circle distance in kilometers.
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	// Convert to radians
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	// Haversine formula
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusKm * c
}

// ParseLocation parses a location string into a Location struct.
// Supports formats:
//   - "region/zone/datacenter" (e.g., "us-east/us-east-1a/dc1")
//   - "region/zone" (e.g., "us-east/us-east-1a")
//   - "region" (e.g., "us-east")
//   - "lat,lon" (e.g., "37.7749,-122.4194")
func ParseLocation(s string) *Location {
	if s == "" {
		return nil
	}

	// Check for coordinate format
	if strings.Contains(s, ",") {
		parts := strings.Split(s, ",")
		if len(parts) == 2 {
			var lat, lon float64
			if _, err := stringToFloat(parts[0], &lat); err == nil {
				if _, err := stringToFloat(parts[1], &lon); err == nil {
					return &Location{
						Latitude:  lat,
						Longitude: lon,
					}
				}
			}
		}
	}

	// Parse hierarchical format
	parts := strings.Split(s, "/")
	loc := &Location{}

	if len(parts) >= 1 {
		loc.Region = parts[0]
	}
	if len(parts) >= 2 {
		loc.Zone = parts[1]
	}
	if len(parts) >= 3 {
		loc.Datacenter = parts[2]
	}

	return loc
}

// stringToFloat is a helper to parse float64.
func stringToFloat(s string, out *float64) (bool, error) {
	s = strings.TrimSpace(s)
	// Simple float parsing without importing strconv in this context
	var val float64
	var neg bool
	var decimal bool
	var decimalPlace float64 = 10.0

	for i, c := range s {
		if c == '-' && i == 0 {
			neg = true
			continue
		}
		if c == '.' {
			if decimal {
				return false, nil // Multiple decimals
			}
			decimal = true
			continue
		}
		if c < '0' || c > '9' {
			return false, nil // Not a digit
		}
		digit := float64(c - '0')
		if decimal {
			val += digit / decimalPlace
			decimalPlace *= 10
		} else {
			val = val*10 + digit
		}
	}

	if neg {
		val = -val
	}
	*out = val
	return true, nil
}

// LocationString returns a string representation of a location.
func LocationString(loc *Location) string {
	if loc == nil {
		return ""
	}

	// If coordinates are set, use them
	if loc.Latitude != 0 || loc.Longitude != 0 {
		return formatCoords(loc.Latitude, loc.Longitude)
	}

	// Otherwise, build hierarchical string
	parts := make([]string, 0)
	if loc.Region != "" {
		parts = append(parts, loc.Region)
	}
	if loc.Zone != "" {
		parts = append(parts, loc.Zone)
	}
	if loc.Datacenter != "" {
		parts = append(parts, loc.Datacenter)
	}

	return strings.Join(parts, "/")
}

func formatCoords(lat, lon float64) string {
	// Simple formatting without strconv
	return formatFloat(lat) + "," + formatFloat(lon)
}

func formatFloat(f float64) string {
	if f == 0 {
		return "0"
	}

	neg := f < 0
	if neg {
		f = -f
	}

	intPart := int(f)
	fracPart := int((f - float64(intPart)) * 10000)

	result := ""
	if neg {
		result = "-"
	}

	// Convert int to string
	if intPart == 0 {
		result += "0"
	} else {
		digits := ""
		for intPart > 0 {
			digits = string(rune('0'+intPart%10)) + digits
			intPart /= 10
		}
		result += digits
	}

	if fracPart > 0 {
		result += "."
		// Add leading zeros if needed
		if fracPart < 1000 {
			result += "0"
		}
		if fracPart < 100 {
			result += "0"
		}
		if fracPart < 10 {
			result += "0"
		}
		// Convert frac to string, removing trailing zeros
		fracStr := ""
		for fracPart > 0 {
			fracStr = string(rune('0'+fracPart%10)) + fracStr
			fracPart /= 10
		}
		// Trim trailing zeros
		for len(fracStr) > 0 && fracStr[len(fracStr)-1] == '0' {
			fracStr = fracStr[:len(fracStr)-1]
		}
		result += fracStr
	}

	return result
}

// DistanceKm returns the distance in kilometers between two locations.
func DistanceKm(a, b *Location) float64 {
	if a == nil || b == nil {
		return -1
	}
	if a.Latitude == 0 && a.Longitude == 0 {
		return -1
	}
	if b.Latitude == 0 && b.Longitude == 0 {
		return -1
	}
	return haversineDistance(a.Latitude, a.Longitude, b.Latitude, b.Longitude)
}

// RegionMatch checks if two locations are in the same region.
func RegionMatch(a, b *Location) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Region != "" && a.Region == b.Region
}

// ZoneMatch checks if two locations are in the same zone.
func ZoneMatch(a, b *Location) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Zone != "" && a.Zone == b.Zone
}

// DatacenterMatch checks if two locations are in the same datacenter.
func DatacenterMatch(a, b *Location) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Datacenter != "" && a.Datacenter == b.Datacenter
}

// InheritLocation creates a new location inheriting missing fields from parent.
func InheritLocation(child, parent *Location) *Location {
	if child == nil {
		return parent
	}
	if parent == nil {
		return child
	}

	result := &Location{
		Region:     child.Region,
		Zone:       child.Zone,
		Datacenter: child.Datacenter,
		Latitude:   child.Latitude,
		Longitude:  child.Longitude,
		Country:    child.Country,
	}

	if result.Region == "" {
		result.Region = parent.Region
	}
	if result.Zone == "" {
		result.Zone = parent.Zone
	}
	if result.Datacenter == "" {
		result.Datacenter = parent.Datacenter
	}
	if result.Latitude == 0 {
		result.Latitude = parent.Latitude
	}
	if result.Longitude == 0 {
		result.Longitude = parent.Longitude
	}
	if result.Country == "" {
		result.Country = parent.Country
	}

	return result
}
