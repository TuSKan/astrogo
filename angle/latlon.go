package angle

// Latitude is an angle north (positive) or south (negative) of a sphere's
// equator: a geodetic latitude, an ecliptic or galactic one, or the latitude
// of any other spherical frame. It carries every method of [Angle].
//
// It is a type of its own rather than an Angle so that a latitude and a
// longitude cannot be passed in each other's place: every astrogo signature
// that takes both takes the latitude first, and a swap does not compile. Build
// one from degrees with [LatDeg], or from an Angle as Latitude{Angle: a}.
//
// A latitude outside ±90° is not refused here; a coordinate built from one
// validates it.
type Latitude struct{ Angle }

// Longitude is an angle measured along a sphere's equator, east positive: a
// geodetic longitude, an ecliptic or galactic one, or the longitude of any
// other spherical frame. It carries every method of [Angle], and no range is
// imposed on it. Build one from degrees with [LonDeg], or from an Angle as
// Longitude{Angle: a}.
type Longitude struct{ Angle }

// LatDeg is the latitude of v degrees.
func LatDeg(v float64) Latitude { return Latitude{Deg(v)} }

// LonDeg is the longitude of v degrees.
func LonDeg(v float64) Longitude { return Longitude{Deg(v)} }
