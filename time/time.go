package time

import (
	"fmt"
	"math"
	"time"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

type Duration = time.Duration

type Location = time.Location

const (
	Second = time.Second
	Minute = time.Minute
	Hour   = time.Hour
)

var LoadLocation = time.LoadLocation

var LocationUTC = time.UTC

var RFC3339 = time.RFC3339

// Scale represents an astronomical time scale.
type Scale uint8

const (
	UTC Scale = iota // Coordinated Universal Time
	TAI              // International Atomic Time
	TT               // Terrestrial Time
	UT1              // Universal Time
	TDB              // Barycentric Dynamical Time
)

func (s Scale) String() string {
	switch s {
	case UTC:
		return "UTC"
	case TAI:
		return "TAI"
	case TT:
		return "TT"
	case UT1:
		return "UT1"
	case TDB:
		return "TDB"
	default:
		return "UNKNOWN"
	}
}

// Time represents a high-precision astronomical timestamp.
//
// Internal representation uses a two-part Julian Date (jd1 + jd2) to maintain
// precision. The split is typically at the nearest day.
type Time struct {
	jd1   float64
	jd2   float64
	scale Scale
}

// ── Constructors ──────────────────────────────────────────────────────────────

// FromJD creates a Time from a single-float Julian Date.
func FromJD(jd float64, s Scale) Time {
	return FromJDParts(jd, 0, s)
}

// FromJDParts creates a Time from a two-part Julian Date.
// It automatically normalizes the components.
func FromJDParts(jd1, jd2 float64, s Scale) Time {
	t := Time{jd1: jd1, jd2: jd2, scale: s}
	t.normalize()
	return t
}

// FromGo creates a Time from a Go standard library time.Time.
// The input is interpreted as being in the UTC scale.
func FromGo(t time.Time) Time {
	utc := t.UTC()
	unix := float64(utc.Unix()) + float64(utc.Nanosecond())/1e9
	// Unix epoch 1970-01-01 is JD 2440587.5
	jd := 2440587.5 + unix/86400.0
	return FromJD(jd, UTC)
}

// Now returns the current time in the UTC scale.
func Now() Time {
	return FromGo(time.Now())
}

// NowUTC returns the current time in the UTC scale.
func NowUTC() Time {
	return Now()
}

// ── Methods ───────────────────────────────────────────────────────────────────

// JD returns the total Julian Date as a single float64.
func (t Time) JD() float64 {
	return t.jd1 + t.jd2
}

// JulianDate is an alias for JD().
func (t Time) JulianDate() float64 {
	return t.JD()
}

// JDParts returns the underlying two-part Julian Date components.
func (t Time) JDParts() (float64, float64) {
	return t.jd1, t.jd2
}

// Scale returns the time scale of the timestamp.
func (t Time) Scale() Scale {
	return t.scale
}

// String returns a simple string representation (JD + Scale).
func (t Time) String() string {
	return fmt.Sprintf("JD %.8f (%s)", t.JD(), t.scale)
}

// ToGo converts the Time to a standard library time.Time.
// The result is in UTC.
func (t Time) ToGo() time.Time {
	// JD 2440587.5 is 1970-01-01 00:00:00 UTC
	unix := (t.JD() - 2440587.5) * 86400.0
	sec := int64(math.Floor(unix))
	nsec := int64((unix - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).UTC()
}

// Add returns a new Time with the duration added.
// It uses a simple conversion: 1 day = 86400.0 seconds.
func (t Time) Add(d time.Duration) Time {
	return t.AddDays(d.Seconds() / 86400.0)
}

// AddDays returns a new Time with d days added.
func (t Time) AddDays(d float64) Time {
	return FromJDParts(t.jd1, t.jd2+d, t.scale)
}

// Date returns a new Time from a Go standard library time.Time.
func Date(year int, month time.Month, day int, hour int, min int, sec int, nsec int, loc *time.Location) Time {
	return FromGo(time.Date(year, month, day, hour, min, sec, nsec, loc))
}

// Format returns a string representation of the time in the given format.
func (t *Time) Format(format string) string {
	return t.ToGo().Format(format)
}

// Before returns true if t is chronologically before other.
// WARNING: This assumes both times are in the same scale for a simple comparison.
func (t Time) Before(other Time) bool {
	if t.jd1 < other.jd1 {
		return true
	}
	if t.jd1 > other.jd1 {
		return false
	}
	return t.jd2 < other.jd2
}

// After returns true if t is chronologically after other.
// WARNING: This assumes both times are in the same scale for a simple comparison.
func (t Time) After(other Time) bool {
	if t.jd1 > other.jd1 {
		return true
	}
	if t.jd1 < other.jd1 {
		return false
	}
	return t.jd2 > other.jd2
}

// Equal reports whether t and other represent the same Julian Date in the same scale.
func (t Time) Equal(other Time) bool {
	return t.scale == other.scale && t.jd1 == other.jd1 && t.jd2 == other.jd2
}

// IsZero reports whether t represents the zero-value Julian Date.
func (t Time) IsZero() bool {
	return t.jd1 == 0 && t.jd2 == 0
}

// Sub returns the duration t - other.
// It uses a simple conversion: 1 day = 86400.0 seconds.
func (t Time) Sub(other Time) time.Duration {
	days := t.SubDays(other)
	return time.Duration(days * 86400.0 * float64(time.Second))
}

// SubDays returns the difference t - other in days.
//
// WARNING: This assumes both times are in the same scale. If they differ, the
// result is currently a simple numerical difference and may be scientifically
// incorrect if scale conversions are ignored.
func (t Time) SubDays(other Time) float64 {
	return (t.jd1 - other.jd1) + (t.jd2 - other.jd2)
}

// ── Internal Helpers ──────────────────────────────────────────────────────────

// TT returns a new Time converted to the Terrestrial Time scale.
func (t Time) TT() Time {
	if t.scale == TT {
		return t
	}
	if t.scale != UTC {
		// Simplified conversion for other scales not implemented in v1
		return t
	}
	// UTC -> TT: TT = UTC + ΔAT + 32.184s
	y, m, d, fd, _ := gofaext.JdToDate(t.jd1, t.jd2)
	dat, _ := gofaext.Dat(y, m, d, fd)
	deltaTT := (dat + 32.184) / 86400.0
	return FromJDParts(t.jd1, t.jd2+deltaTT, TT)
}

// TDB returns a new Time converted to the Barycentric Dynamical Time scale.
// In this implementation, TDB is approximated as being equal to TT.
func (t Time) TDB() Time {
	if t.scale == TDB {
		return t
	}
	tt := t.TT()
	return Time{jd1: tt.jd1, jd2: tt.jd2, scale: TDB}
}

// normalize ensures that |jd2| < 1.0, and both components are properly balanced.
func (t *Time) normalize() {
	if math.IsNaN(t.jd1) || math.IsNaN(t.jd2) {
		return
	}
	// Move integer days from jd2 to jd1
	extraDays := math.Floor(t.jd2)
	t.jd1 += extraDays
	t.jd2 -= extraDays

	// If jd1 is not an integer (user passed fractional jd1), move its fraction to jd2
	jd1Int, jd1Frac := math.Modf(t.jd1)
	t.jd1 = jd1Int
	t.jd2 += jd1Frac

	// Final pass: ensure -0.5 <= jd2 < 0.5 for stability?
	// standard astro libraries often use:
	//   jd1 = integer + 0.5 (JD starts at noon)
	//   jd2 = fraction of day
	// But simple "integer + remainder" is often more robust for general diffs.
	// We'll stick to simple "floor" normalization for now.
}
