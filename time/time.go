package time

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/TuSKan/astrogo/iers"
	"github.com/TuSKan/astrogo/internal/gofaext"
)

var warnUT1Once sync.Once

type Duration = time.Duration

type Location = time.Location

type Month = time.Month

const (
	Second = time.Second
	Minute = time.Minute
	Hour   = time.Hour
)

// Month constants re-exported from the standard library.
const (
	January   = time.January
	February  = time.February
	March     = time.March
	April     = time.April
	May       = time.May
	June      = time.June
	July      = time.July
	August    = time.August
	September = time.September
	October   = time.October
	November  = time.November
	December  = time.December
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
//
// An optional display location (loc) is carried for presentation purposes.
// It affects only [Time.ToGo], [Time.Format], and [Time.String] and never
// influences scientific computation or scale conversions.
type Time struct {
	jd1   float64
	jd2   float64
	scale Scale
	loc   *time.Location // display-only; nil defaults to UTC
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
// The original time.Location is preserved for display purposes.
func FromGo(t time.Time) Time {
	loc := t.Location()
	utc := t.UTC()
	unixSec := float64(utc.Unix())
	unixNsec := float64(utc.Nanosecond()) / 1e9

	// Unix epoch 1970-01-01 is JD 2440587.5
	days := math.Floor(unixSec / 86400.0)
	frac := (unixSec-days*86400.0)/86400.0 + unixNsec/86400.0

	result := FromJDParts(2440587.5+days, frac, UTC)
	result.loc = loc
	return result
}

// NowUTC returns the current time in the UTC scale.
func NowUTC() Time {
	return FromGo(time.Now())
}

func ZeroTime() Time {
	return FromJD(0, UTC)
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

// String returns a human-readable representation.
// If a display location is set, the civil time in that timezone is shown;
// otherwise the raw JD and scale are returned.
func (t Time) String() string {
	if t.loc != nil && t.loc != time.UTC {
		return t.ToGo().Format("2006-01-02 15:04:05 MST")
	}
	return fmt.Sprintf("JD %.8f (%s)", t.JD(), t.scale)
}

// ToGo converts the Time to a standard library time.Time.
// If a display location was set (via [FromGo], [Date], or [Time.In]),
// the result is expressed in that timezone; otherwise it defaults to UTC.
func (t Time) ToGo() time.Time {
	// JD 2440587.5 is 1970-01-01 00:00:00 UTC
	days1 := t.jd1 - 2440587.5
	days2 := t.jd2

	totalSec := days1*86400.0 + days2*86400.0

	// Round to the nearest nanosecond to avoid floating-point drift
	nsecTotal := int64(math.Round(totalSec * 1e9))
	sec := nsecTotal / 1e9
	nsec := nsecTotal % 1e9
	if nsec < 0 {
		sec -= 1
		nsec += 1e9
	}
	gt := time.Unix(sec, nsec).UTC()
	if t.loc != nil {
		return gt.In(t.loc)
	}
	return gt
}

// GoTime is an alias for [ToGo].
func (t Time) GoTime() time.Time { return t.ToGo() }

// Year returns the Gregorian calendar year of t.
func (t Time) Year() int { return t.ToGo().Year() }

// Add returns a new Time with the duration added.
// It uses a simple conversion: 1 day = 86400.0 seconds.
// The display location is preserved.
func (t Time) Add(d time.Duration) Time {
	return t.AddDays(d.Seconds() / 86400.0)
}

// AddDays returns a new Time with d days added.
// The display location is preserved.
func (t Time) AddDays(d float64) Time {
	result := FromJDParts(t.jd1, t.jd2+d, t.scale)
	result.loc = t.loc
	return result
}

// Date creates a Time from calendar components and a timezone.
// The timezone location is preserved for display purposes.
func Date(year int, month time.Month, day int, hour int, min int, sec int, nsec int, loc *time.Location) Time {
	return FromGo(time.Date(year, month, day, hour, min, sec, nsec, loc))
}

// Location returns the display timezone associated with this Time.
// Returns time.UTC if none was set.
func (t Time) Location() *time.Location {
	if t.loc != nil {
		return t.loc
	}
	return time.UTC
}

// In returns a copy of t with the display location set to loc.
// This does not change the underlying instant; only how it is displayed
// by [Time.ToGo], [Time.Format], and [Time.String].
func (t Time) In(loc *time.Location) Time {
	t.loc = loc
	return t
}

// Format returns a string representation of the time in the given layout.
// The display location is applied before formatting.
func (t Time) Format(format string) string {
	return t.ToGo().Format(format)
}

// Before reports whether t is chronologically before other.
// If t and other are in different time scales, both are automatically
// converted to TT for comparison. Same-scale comparisons have zero overhead.
func (t Time) Before(other Time) bool {
	if t.scale != other.scale {
		t, other = t.TT(), other.TT()
	}
	if t.jd1 < other.jd1 {
		return true
	}
	if t.jd1 > other.jd1 {
		return false
	}
	return t.jd2 < other.jd2
}

// After reports whether t is chronologically after other.
// Cross-scale times are automatically unified via TT.
func (t Time) After(other Time) bool {
	if t.scale != other.scale {
		t, other = t.TT(), other.TT()
	}
	if t.jd1 > other.jd1 {
		return true
	}
	if t.jd1 < other.jd1 {
		return false
	}
	return t.jd2 > other.jd2
}

// Equal reports whether t and other represent the same physical instant.
// Cross-scale times are automatically unified via TT.
func (t Time) Equal(other Time) bool {
	if t.scale != other.scale {
		t, other = t.TT(), other.TT()
	}
	return t.jd1 == other.jd1 && t.jd2 == other.jd2
}

// IsZero reports whether t represents the zero-value Julian Date.
func (t Time) IsZero() bool {
	return t.jd1 == 0 && t.jd2 == 0
}

// Sub returns the duration t - other.
// Cross-scale times are automatically unified via TT.
func (t Time) Sub(other Time) time.Duration {
	days := t.SubDays(other)
	return time.Duration(days * 86400.0 * float64(time.Second))
}

// SubDays returns the difference t - other in days.
// Cross-scale times are automatically unified via TT.
func (t Time) SubDays(other Time) float64 {
	if t.scale != other.scale {
		t, other = t.TT(), other.TT()
	}
	return (t.jd1 - other.jd1) + (t.jd2 - other.jd2)
}

// ── Scale Conversion Helpers ─────────────────────────────────────────────────

// fromPartsPreserveLoc creates a Time in the given scale from a two-part JD,
// preserving the display location of the source time.
func fromPartsPreserveLoc(src Time, jd1, jd2 float64, s Scale) Time {
	result := FromJDParts(jd1, jd2, s)
	result.loc = src.loc
	return result
}

// tdbMinusTT returns the TDB−TT difference in seconds for a given epoch,
// using the single-term Fairhead & Bretagnon (1990) approximation.
//
// The dominant sinusoidal term has amplitude 1.657 ms and period ≈1 year.
// This covers >99.5% of the total TDB−TT variation; higher-order terms
// contribute <3 μs and are negligible for all astrogo use cases.
//
// Reference: Fairhead L., Bretagnon P., A&A 229, 240 (1990).
func tdbMinusTT(jdTT1, jdTT2 float64) float64 {
	// T = Julian centuries from J2000.0 TT
	T := ((jdTT1 - 2451545.0) + jdTT2) / 36525.0
	// Mean anomaly of Earth (degrees → radians)
	g := (357.5277233 + 35999.0503400*T) * (math.Pi / 180.0)
	return 0.001657 * math.Sin(g)
}

// dut1ForUTC retrieves DUT1 for the given UTC two-part JD.
// Returns (dut1, nil) on success, or (0, err) if IERS data is unavailable.
func dut1ForUTC(jd1, jd2 float64) (float64, error) {
	mjd := (jd1 - 2400000.5) + jd2
	eop, err := iers.GetModel().EOP(mjd)
	if err != nil {
		return 0, err
	}
	return eop.DUT1, nil
}

// dut1OrFallback retrieves DUT1 with a fallback to 0.0 on error.
// Logs a one-time warning when falling back.
func dut1OrFallback(jd1, jd2 float64) float64 {
	dut1, err := dut1ForUTC(jd1, jd2)
	if err != nil {
		warnUT1Once.Do(func() {
			mjd := (jd1 - 2400000.5) + jd2
			log.Printf("astrogo/time: IERS EOP data unavailable (MJD %.1f): UT1 ≈ UTC (DUT1=0). Max error ≈ 0.9s. Load finals2000A for sub-second precision.", mjd)
		})
		return 0
	}
	return dut1
}

// ── Scale Conversions ────────────────────────────────────────────────────────
//
// Conversion graph:
//
//	UTC ←→ TAI ←→ TT ←→ TDB
//	 ↕
//	UT1
//
// All conversions except those involving UT1 are deterministic.
// UT1 depends on IERS Earth Orientation Parameters (DUT1).

// UTC returns a new Time converted to the Coordinated Universal Time scale.
//
// For UT1 input, the conversion uses IERS DUT1 data when available,
// falling back to DUT1=0 (max error 0.9s) with a one-time log warning.
func (t Time) UTC() Time {
	if t.scale == UTC {
		return t
	}
	switch t.scale {
	case TAI:
		// UTC = TAI − ΔAT.
		// Use TAI JD as initial UTC guess for the leap-second lookup,
		// then iterate once to handle the leap-second boundary.
		y, m, d, fd, _ := gofaext.JdToDate(t.jd1, t.jd2)
		dat, _ := gofaext.Dat(y, m, d, fd)
		utcJD2 := t.jd2 - dat/86400.0
		// Re-check: ΔAT may differ at the true UTC epoch (leap-second edge).
		y2, m2, d2, fd2, _ := gofaext.JdToDate(t.jd1, utcJD2)
		dat2, _ := gofaext.Dat(y2, m2, d2, fd2)
		if dat2 != dat {
			utcJD2 = t.jd2 - dat2/86400.0
		}
		return fromPartsPreserveLoc(t, t.jd1, utcJD2, UTC)
	case TT:
		// TT → TAI → UTC: TAI = TT − 32.184s
		tai := fromPartsPreserveLoc(t, t.jd1, t.jd2-32.184/86400.0, TAI)
		return tai.UTC()
	case TDB:
		// TDB → TT → TAI → UTC
		tt := fromPartsPreserveLoc(t, t.jd1, t.jd2-tdbMinusTT(t.jd1, t.jd2)/86400.0, TT)
		return tt.UTC()
	case UT1:
		// UTC = UT1 − DUT1. Since |DUT1| < 0.9s, UT1 ≈ UTC for lookup.
		dut1 := dut1OrFallback(t.jd1, t.jd2)
		return fromPartsPreserveLoc(t, t.jd1, t.jd2-dut1/86400.0, UTC)
	}
	return t // unreachable with current scales
}

// TAI returns a new Time converted to the International Atomic Time scale.
func (t Time) TAI() Time {
	if t.scale == TAI {
		return t
	}
	switch t.scale {
	case UTC:
		// TAI = UTC + ΔAT
		y, m, d, fd, _ := gofaext.JdToDate(t.jd1, t.jd2)
		dat, _ := gofaext.Dat(y, m, d, fd)
		return fromPartsPreserveLoc(t, t.jd1, t.jd2+dat/86400.0, TAI)
	case TT:
		// TAI = TT − 32.184s
		return fromPartsPreserveLoc(t, t.jd1, t.jd2-32.184/86400.0, TAI)
	default:
		// TDB, UT1 → UTC → TAI
		return t.UTC().TAI()
	}
}

// TT returns a new Time converted to the Terrestrial Time scale.
//
// All conversion paths are deterministic. For UT1 input, the internal
// UTC conversion may use a DUT1 fallback (DUT1=0, max error 0.9s)
// if IERS data is unavailable.
func (t Time) TT() Time {
	if t.scale == TT {
		return t
	}
	switch t.scale {
	case UTC:
		// TT = UTC + ΔAT + 32.184s
		y, m, d, fd, _ := gofaext.JdToDate(t.jd1, t.jd2)
		dat, _ := gofaext.Dat(y, m, d, fd)
		return fromPartsPreserveLoc(t, t.jd1, t.jd2+(dat+32.184)/86400.0, TT)
	case TAI:
		// TT = TAI + 32.184s
		return fromPartsPreserveLoc(t, t.jd1, t.jd2+32.184/86400.0, TT)
	case TDB:
		// TT = TDB − (TDB−TT). Use TDB JD as TT approximation for the
		// correction term (residual error ≈ 4 ns, negligible).
		return fromPartsPreserveLoc(t, t.jd1, t.jd2-tdbMinusTT(t.jd1, t.jd2)/86400.0, TT)
	case UT1:
		// UT1 → UTC (with fallback) → TT
		return t.UTC().TT()
	}
	return t // unreachable
}

// TDB returns a new Time converted to the Barycentric Dynamical Time scale.
//
// Uses the single-term Fairhead & Bretagnon (1990) approximation for the
// TDB−TT correction (amplitude 1.657 ms, period ≈1 year, >99.5% of signal).
func (t Time) TDB() Time {
	if t.scale == TDB {
		return t
	}
	tt := t.TT()
	correction := tdbMinusTT(tt.jd1, tt.jd2) / 86400.0
	return fromPartsPreserveLoc(t, tt.jd1, tt.jd2+correction, TDB)
}

// UT1 returns a new Time converted to the Universal Time (UT1) scale.
//
// This conversion requires IERS Earth Orientation Parameters for DUT1.
// Returns an error if IERS data is unavailable for the given epoch.
// The embedded finals2000A data is auto-loaded when the iers package is
// imported; for dates beyond its prediction window, load updated data
// via [iers.RegisterModel].
func (t Time) UT1() (Time, error) {
	if t.scale == UT1 {
		return t, nil
	}
	utc := t.UTC() // deterministic route to UTC
	dut1, err := dut1ForUTC(utc.jd1, utc.jd2)
	if err != nil {
		return Time{}, fmt.Errorf("astrogo/time: UT1 conversion failed (MJD %.1f): %w",
			(utc.jd1-2400000.5)+utc.jd2, err)
	}
	return fromPartsPreserveLoc(t, utc.jd1, utc.jd2+dut1/86400.0, UT1), nil
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

	// Re-normalize in case jd2 overflowed past 1.0 (e.g. 0.5 + 0.5)
	extraDays = math.Floor(t.jd2)
	t.jd1 += extraDays
	t.jd2 -= extraDays
}
