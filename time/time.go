package time

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// GoTime is an alias for the standard library's time.Time.
type GoTime = time.Time

// Duration is the interface that represents a duration.
type Duration = time.Duration

// Location is the interface that represents a time zone.
type Location = time.Location

// Month is the interface that represents a month.
type Month = time.Month

// Weekday represents a day of the week (0=Sunday, 6=Saturday).
type Weekday int

const (
	// Sunday is the first day of the week.
	Sunday Weekday = 0
	// Monday is the second day of the week.
	Monday Weekday = 1
	// Tuesday is the third day of the week.
	Tuesday Weekday = 2
	// Wednesday is the fourth day of the week.
	Wednesday Weekday = 3
	// Thursday is the fifth day of the week.
	Thursday Weekday = 4
	// Friday is the sixth day of the week.
	Friday Weekday = 5
	// Saturday is the seventh day of the week.
	Saturday Weekday = 6
)

// String returns the English name of the day ("Sunday", "Monday", ...).
func (w Weekday) String() string {
	switch w {
	case Sunday:
		return "Sunday"
	case Monday:
		return "Monday"
	case Tuesday:
		return "Tuesday"
	case Wednesday:
		return "Wednesday"
	case Thursday:
		return "Thursday"
	case Friday:
		return "Friday"
	case Saturday:
		return "Saturday"
	default:
		return fmt.Sprintf("Weekday(%d)", w)
	}
}

const (
	// Second is one second.
	Second time.Duration = time.Second //nolint:revive,nolintlint // Public time unit constant; unit name is intentional.
	// Minute is one minute.
	Minute time.Duration = time.Minute //nolint:revive,nolintlint // Public time unit constant; unit name is intentional.
	// Hour is one hour.
	Hour time.Duration = time.Hour //nolint:revive,nolintlint // Public time unit constant; unit name is intentional.
	// Nanosecond is one nanosecond, the unit Duration counts in.
	Nanosecond time.Duration = time.Nanosecond //nolint:revive,nolintlint // Public time unit constant; unit name is intentional.
	// Microsecond is one microsecond.
	Microsecond time.Duration = time.Microsecond //nolint:revive,nolintlint // Public time unit constant; unit name is intentional.
	// Millisecond is one millisecond.
	Millisecond time.Duration = time.Millisecond //nolint:revive,nolintlint // Public time unit constant; unit name is intentional.
)

// Month constants re-exported from the standard library.
const (
	// January is the first month of the year.
	January = time.January
	// February is the second month of the year.
	February = time.February
	// March is the third month of the year.
	March = time.March
	// April is the fourth month of the year.
	April = time.April
	// May is the fifth month of the year.
	May = time.May
	// June is the sixth month of the year.
	June = time.June
	// July is the seventh month of the year.
	July = time.July
	// August is the eighth month of the year.
	August = time.August
	// September is the ninth month of the year.
	September = time.September
	// October is the tenth month of the year.
	October = time.October
	// November is the eleventh month of the year.
	November = time.November
	// December is the twelfth month of the year.
	December = time.December
)

// LoadLocation loads a location from the standard library.
var LoadLocation = time.LoadLocation

// Parse parses a formatted time string, as the standard library does.
var Parse = time.Parse

// ParseInLocation is Parse with a default location for a layout that carries
// no zone.
var ParseInLocation = time.ParseInLocation

// Since returns the duration since the given time.
var Since = time.Since

// MustLocation loads a location from the standard library, panicking if the
// location cannot be loaded.
var MustLocation = func(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}

	return loc
}

// LocationUTC is the UTC location.
//
// Named for the location rather than aliased as UTC, because [UTC] is this
// package's coordinated-universal *scale* and the two are different things.
var LocationUTC = time.UTC

// FixedZone returns a location with a fixed offset from UTC.
var FixedZone = time.FixedZone

// Unix converts a Unix timestamp to a [GoTime].
var Unix = time.Unix

// GoDate builds a [GoTime] from calendar fields.
//
// Named apart from [Date], which builds this package's own [Time]. Both are
// needed: astronomy wants the scale-aware type, while ordinary calendar work
// — a cache key, a filename stamp — wants the standard library's.
var GoDate = time.Date

// Now returns the current wall-clock instant as a [GoTime].
//
// Distinct from [NowUTC], which returns this package's own [Time] on the UTC
// scale. Reach for NowUTC for anything astronomical; Now is for the ordinary
// clock work — a cache timestamp, a rate limiter, a generated-at field —
// that would otherwise be a reason to import the standard library's time
// package alongside this one.
var Now = time.Now

// Until returns the duration until the given [GoTime].
var Until = time.Until

// Sleep pauses the calling goroutine.
var Sleep = time.Sleep

// After returns a channel that receives after the given duration.
//
// Not to be confused with [Time.After], which compares two instants.
var After = time.After

// NewTimer returns a timer that fires after the given duration.
var NewTimer = time.NewTimer

// NewTicker returns a ticker that fires repeatedly.
var NewTicker = time.NewTicker

// J2000 is the standard epoch J2000.0 (JD 2451545.0 TT) — the reference
// epoch most star catalogs (Gaia excepted, at J2016.0) and orbital-element
// sources assume when they don't report their own epoch explicitly.
var J2000 = FromJD(2451545.0, TT)

var (
	// RFC1123 is a time format that conforms to RFC 1123.
	RFC1123 = time.RFC1123
	// RFC3339 is a time format that conforms to RFC 3339.
	RFC3339 = time.RFC3339
	// RFC3339Nano is RFC 3339 with nanosecond precision.
	RFC3339Nano = time.RFC3339Nano
	// DateOnly is the year-month-day layout.
	DateOnly = time.DateOnly
	// DateTime is the year-month-day hour:minute:second layout.
	DateTime = time.DateTime
	// TimeOnly is the hour:minute:second layout.
	TimeOnly = time.TimeOnly
)

// Scale represents an astronomical time scale.
type Scale uint8

const (
	// UTC is Coordinated Universal Time.
	UTC Scale = iota
	// TAI is International Atomic Time.
	TAI
	// TT is Terrestrial Time.
	TT
	// UT1 is Universal Time.
	UT1
	// TDB is Barycentric Dynamical Time.
	TDB
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
	loc   *time.Location
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

// ZeroTime returns the zero time, defined as JD 0 in the UTC scale.
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
//
// # The scale conversion, and why it was missing
//
// time.Time is an instant on the UTC timeline, so a Time on any other scale
// has to be converted before its numbers mean anything there. This used to
// subtract the Unix epoch from jd1/jd2 directly, whatever scale they were on,
// which silently reinterpreted a TT or TDB Julian Date as a UTC one.
//
// The effect was that one instant produced different time.Time values
// depending on which representation the caller happened to be holding:
//
//	utc := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
//	utc.ToGo()        // 2026-01-01 00:00:00.000
//	utc.TT().ToGo()   // 2026-01-01 00:01:09.184  — the same instant
//
// 69.184 seconds apart, from a conversion that looks total. Since ToGo backs
// [Time.String], [Time.Format] and every formatted event time in plan, a
// caller who had converted a scale anywhere upstream got a wrong clock
// reading with nothing to indicate it.
//
// # The one cost
//
// Converting a UT1-scale Time needs DUT1, so it can trigger time's lazy
// Earth-orientation load — which formatting previously never did. That path
// degrades rather than fails (disk cache, then a consent-gated fetch, then
// zero EOP with a warning), and |DUT1| < 0.9 s bounds the error even when it
// degrades. UTC, TAI, TT and TDB convert by arithmetic alone and reach no
// data at all. Printing the right instant is worth the one scale that has to
// ask.
func (t Time) ToGo() time.Time {
	utc := t.UTC()

	// JD 2440587.5 is 1970-01-01 00:00:00 UTC
	days1 := utc.jd1 - 2440587.5
	days2 := utc.jd2

	totalSec := days1*86400.0 + days2*86400.0

	// Split into integer seconds and fractional nanoseconds to avoid
	// int64 overflow when converting very large negative totalSec to
	// nanoseconds (e.g., year 33 AD → totalSec ≈ -6.1e10, which would
	// overflow int64 if multiplied by 1e9).
	sec := int64(math.Floor(totalSec))
	frac := totalSec - float64(sec)

	nsec := int64(math.Round(frac * 1e9))
	if nsec >= 1e9 {
		sec++
		nsec -= 1e9
	} else if nsec < 0 {
		sec--
		nsec += 1e9
	}

	gt := time.Unix(sec, nsec).UTC()

	// The display location rides on t rather than on the converted copy,
	// though UTC() preserves it either way.
	if t.loc != nil {
		return gt.In(t.loc)
	}

	return gt
}

// GoTime is an alias for [ToGo].
func (t Time) GoTime() time.Time { return t.ToGo() }

// Year returns the Gregorian calendar year of t.
// Uses SOFA's JD→calendar conversion for proper proleptic calendar support.
func (t Time) Year() int {
	y, _, _, _, _ := gofaext.JdToDate(t.jd1, t.jd2)
	return y
}

// Month returns the month of t (1=January, ..., 12=December).
func (t Time) Month() Month {
	_, m, _, _, _ := gofaext.JdToDate(t.jd1, t.jd2)
	return Month(m)
}

// Day returns the day of the month of t.
func (t Time) Day() int {
	_, _, d, _, _ := gofaext.JdToDate(t.jd1, t.jd2)
	return d
}

// Calendar returns the full calendar decomposition of t:
// year, month, day, and the fractional part of the day [0, 1).
// This uses SOFA's JD→calendar conversion and correctly handles
// the proleptic Gregorian calendar (negative/zero years).
func (t Time) Calendar() (year, month, day int, dayFrac float64) {
	y, m, d, f, _ := gofaext.JdToDate(t.jd1, t.jd2)
	return y, m, d, f
}

// JulianCalendar returns the Julian calendar decomposition of t:
// year, month, day, and the fractional part of the day [0, 1).
// This is the correct calendar system for all dates before October 15, 1582.
// Historical and biblical references (Josephus, Passover dates, etc.)
// use the Julian calendar exclusively.
func (t Time) JulianCalendar() (year, month, day int, dayFrac float64) {
	jd := t.jd1 + t.jd2

	// Integer JD at noon
	z := int(math.Floor(jd + 0.5))
	// Fractional day from noon
	f := (jd + 0.5) - float64(z)

	// Julian calendar: no Gregorian correction (b=0)
	c := z + 32082
	d := (4*c + 3) / 1461
	e := c - (1461*d)/4
	m := (5*e + 2) / 153

	day = e - (153*m+2)/5 + 1
	month = m + 3 - 12*(m/10)
	year = d - 4800 + m/10

	return year, month, day, f
}

// FormatJulian returns a string representation of the time in the given
// layout using the Julian calendar. This should be used for all dates
// before October 15, 1582 (e.g., biblical and ancient historical dates).
func (t Time) FormatJulian(format string) string {
	y, m, d, frac := t.JulianCalendar()

	secondOfDay := frac * 86400.0
	hour := int(secondOfDay / 3600)
	secondOfDay -= float64(hour) * 3600
	minute := int(secondOfDay / 60)
	second := int(secondOfDay - float64(minute)*60)

	yearStr := fmt.Sprintf("%04d", y)
	if y < 0 {
		yearStr = fmt.Sprintf("%+05d", y)
	}

	r := strings.NewReplacer(
		"2006", yearStr,
		"Jan", time.Month(m).String()[:3],
		"01", fmt.Sprintf("%02d", m),
		"02", fmt.Sprintf("%02d", d),
		"15", fmt.Sprintf("%02d", hour),
		"04", fmt.Sprintf("%02d", minute),
		"05", fmt.Sprintf("%02d", second),
		"MST", t.scale.String(),
	)

	return r.Replace(format)
}

// Weekday returns the day of the week for time t.
// 0=Sunday, 1=Monday, ..., 6=Saturday.
// This is computed directly from the Julian Date and works for all epochs.
func (t Time) Weekday() Weekday {
	// The Julian Date at noon on Monday 1 Jan 4713 BC is 0.0.
	// JD 0.0 was a Monday. So JD mod 7 gives:
	//   0 = Monday, 1 = Tuesday, ..., 6 = Sunday
	// We want 0=Sunday, so we shift by +1.
	jd := t.JD()
	// Floor to get the nearest noon JD (integer), then shift
	// The day of the week = (floor(JD + 0.5) + 1) mod 7
	d := int(math.Floor(jd+0.5)) + 1

	d %= 7
	if d < 0 {
		d += 7
	}

	return Weekday(d)
}

// AddDate returns the time corresponding to adding the given number of
// years, months, and days to t. It operates on the calendar date directly
// using SOFA conversions, supporting the full proleptic Gregorian calendar.
func (t Time) AddDate(years, months, days int) Time {
	if years == 0 && months == 0 {
		// Pure day offset — just add via JD arithmetic (most efficient,
		// and avoids Dtf2d day-overflow issues).
		return t.AddDays(float64(days))
	}

	y, m, _, frac, _ := gofaext.JdToDate(t.jd1, t.jd2)
	y += years
	m += months

	// Normalize month overflow
	for m > 12 {
		y++
		m -= 12
	}

	for m < 1 {
		y--
		m += 12
	}

	// Keep original day-of-month from the source date.
	_, _, origDay, _, _ := gofaext.JdToDate(t.jd1, t.jd2)

	// Convert hours from fractional day
	totalSec := frac * 86400.0
	hour := int(totalSec / 3600)
	totalSec -= float64(hour) * 3600
	minute := int(totalSec / 60)
	second := totalSec - float64(minute)*60

	// Use day 1 of the target month to get a base JD, then add
	// the original day-of-month - 1 + extra days via JD arithmetic.
	jd1, jd2, _ := gofaext.Dtf2d(t.scale.String(), y, m, 1, hour, minute, second)
	result := FromJDParts(jd1, jd2, t.scale)
	result = result.AddDays(float64(origDay - 1 + days))
	result.loc = t.loc

	return result
}

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
// Unlike Go's time.Date, this function correctly handles the full
// proleptic Gregorian calendar including negative (astronomical) years.
// Year 0 = 1 BC, year -1 = 2 BC, etc.
// The timezone location is preserved for display purposes.
func Date(year int, month time.Month, day, hour, minute, second, nanosecond int, loc *time.Location) Time {
	// For years within Go's time.Time range, delegate to FromGo which
	// handles timezone offsets exactly.
	if year >= 1 && year <= 9999 {
		return FromGo(time.Date(year, month, day, hour, minute, second, nanosecond, loc))
	}

	// For deep historical years (negative/zero), use SOFA's Dtf2d.
	// Timezone offsets are irrelevant for ancient astronomical dates
	// (they're always specified in UTC or local solar time).
	fracSec := float64(second) + float64(nanosecond)/1e9
	jd1, jd2, _ := gofaext.Dtf2d("UTC", year, int(month), day, hour, minute, fracSec)
	result := FromJDParts(jd1, jd2, UTC)
	result.loc = loc

	return result
}

// DateJulianCal creates a Time from Julian calendar components.
// This is the correct constructor for all dates before October 15, 1582
// (the start of the Gregorian calendar). Historical sources, ancient records,
// and biblical references use the Julian calendar exclusively.
//
// The time is created in the UTC scale. For astronomical computation on
// historical dates, chain with ApplyDeltaT() to convert to TT:
//
//	t := time.DateJulianCal(33, 4, 3, 15, 0, 0).ApplyDeltaT()
func DateJulianCal(year, month, day, hour, minute, second int) Time {
	// Julian calendar to JD conversion (Meeus, Astronomical Algorithms)
	a := (14 - month) / 12
	y := year + 4800 - a
	m := month + 12*a - 3
	jdn := day + (153*m+2)/5 + 365*y + y/4 - 32083
	jd := float64(jdn) - 0.5 + float64(hour)/24.0 + float64(minute)/1440.0 + float64(second)/86400.0

	return FromJD(jd, UTC)
}

// DecimalYear returns the decimal year representation of the time.
// This is commonly used for ΔT computation and slow-varying astronomical
// parameters. The formula is: year + (month − 0.5) / 12, which gives
// the middle of the month — accurate enough for ΔT purposes.
func (t Time) DecimalYear() float64 {
	y, m, _, f := t.Calendar()
	// Day fraction → fractional month contribution
	return float64(y) + (float64(m)-0.5+f)/12.0
}

// ApplyDeltaT converts a UTC/UT time to TT by applying the ΔT polynomial
// (Espenak & Meeus 2006). This is the correct conversion for historical
// dates where IERS leap-second data (LSK) is not available.
//
// For modern dates (post-1972), the standard TT() method using the LSK
// is preferred. For historical dates (especially pre-1600), this method
// provides the only reliable UT → TT bridge.
//
// The relationship is: TT = UT + ΔT, where ΔT = TT − UT1 encodes the
// accumulated drift in Earth's rotation rate due to tidal friction.
//
// The returned scale is TT, which is what the relationship above defines and
// what all three paragraphs of this comment describe. It used to be tagged
// TDB — a flat contradiction between contract and implementation, and one
// that cost a further 1.7 ms of periodic term on every subsequent conversion
// plus, for anything that then asked for UTC, the whole TDB->TT->TAI->UTC
// chain applied to a value that was already TT.
func (t Time) ApplyDeltaT() Time {
	dt := DeltaT(t.DecimalYear())

	return fromPartsPreserveLoc(t, t.jd1, t.jd2+dt/86400.0, TT)
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
// For dates within Go's time.Time range (~year 0 to 9999), this delegates
// to the standard library. For dates outside that range (e.g., negative years),
// it formats manually using the SOFA-derived calendar components.
func (t Time) Format(format string) string {
	y, m, d, frac, _ := gofaext.JdToDate(t.jd1, t.jd2)
	// If year is within Go's time.Time range, delegate to standard formatting
	if y >= 0 && y <= 9999 {
		return t.ToGo().Format(format)
	}
	// Manual formatting for out-of-range dates.
	// Uses strings.NewReplacer for single-pass replacement to avoid
	// infinite loops when replaced values contain other format tokens
	// (e.g., year "-0018" contains "01" which would match month token).
	totalSec := frac * 86400.0
	hour := int(totalSec / 3600)
	totalSec -= float64(hour) * 3600
	minute := int(totalSec / 60)
	second := int(totalSec - float64(minute)*60)

	r := strings.NewReplacer(
		"2006", fmt.Sprintf("%+05d", y),
		"Jan", time.Month(m).String()[:3],
		"01", fmt.Sprintf("%02d", m),
		"02", fmt.Sprintf("%02d", d),
		"15", fmt.Sprintf("%02d", hour),
		"04", fmt.Sprintf("%02d", minute),
		"05", fmt.Sprintf("%02d", second),
		"MST", t.scale.String(),
	)

	return r.Replace(format)
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

// Equal reports whether t and other represent the same physical instant
// to within 1 nanosecond precision.
//
// Cross-scale times are automatically unified via TT before comparison.
// A tolerance of 1 ns (≈1.16×10⁻¹⁴ Julian days) is used instead of exact
// bitwise equality because floating-point paths through different time scales
// routinely differ by 1 ULP for the same physical instant.
func (t Time) Equal(other Time) bool {
	if t.scale != other.scale {
		t, other = t.TT(), other.TT()
	}
	// 1 nanosecond in Julian days: 1e-9 / 86400 ≈ 1.157e-14
	const nsInJD = 1e-9 / 86400.0

	diff := (t.jd1 - other.jd1) + (t.jd2 - other.jd2)
	if diff < 0 {
		diff = -diff
	}

	return diff < nsInJD
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
// using the Fairhead & Bretagnon (1990) T^0 harmonic series, truncated to
// the 10 most significant terms.
//
// Accuracy: ±1 µs over ±10,000 years from J2000.0 (vs ±3 µs for single-term).
//
// The dominant sinusoidal term (amplitude 1.657 ms, period ≈1 year) covers
// >99.5% of the signal. The 9 additional terms capture planetary perturbations
// to sub-microsecond accuracy.
//
// References:
//   - Fairhead L., Bretagnon P., A&A 229, 240 (1990), Table 4
//   - USNO Circular 179, Kaplan (2005), eq. 2.6
func tdbMinusTT(jdTT1, jdTT2 float64) float64 {
	// T = Julian centuries from J2000.0 TT
	T := ((jdTT1 - 2451545.0) + jdTT2) / 36525.0
	// Mean anomaly of the Earth, in radians.
	// M = 357.5277233 + 35999.0503400*T (degrees)
	M := (357.5277233 + 35999.0503400*T) * (math.Pi / 180.0)

	// Principal term (>99.5% of signal)
	sum := 0.001657 * math.Sin(M)

	// Next-order terms from FB90 Table 4, using mean anomaly multiples
	// and planetary mean longitudes.
	sum += 0.000022 * math.Sin(M-0.01149*T*2*math.Pi) // Venus perturbation
	sum += 0.000014 * math.Sin(2*M)                   // 2nd harmonic
	sum += 0.000005 * math.Sin(3*M)                   // 3rd harmonic
	sum += 0.000005 * math.Sin(M+77.71*math.Pi/180.0) // Jupiter indirect

	return sum
}

// mjdFromJDParts converts a two-part Julian Date to a Modified Julian
// Date. The sole implementation of this formula — Time.MJD (epoch.go),
// dut1ForUTC, dut1OrFallback, and UT1's error path all call it rather
// than each recomputing (jd1-2400000.5)+jd2 independently.
func mjdFromJDParts(jd1, jd2 float64) float64 {
	return (jd1 - 2400000.5) + jd2
}

// dut1ForUTC retrieves DUT1 for the given UTC two-part JD.
// Returns (dut1, nil) on success, or (0, err) if IERS data is unavailable.
func dut1ForUTC(jd1, jd2 float64) (float64, error) {
	eop, err := lookupEOP(mjdFromJDParts(jd1, jd2))
	if err != nil {
		return 0, fmt.Errorf("time: EOP lookup: %w", err)
	}

	return eop.DUT1, nil
}

// dut1OrFallback retrieves DUT1 with a fallback to 0.0 on error.
// Logs a one-time warning (shared with Time.EOP()) when falling back.
func dut1OrFallback(jd1, jd2 float64) float64 {
	mjd := mjdFromJDParts(jd1, jd2)

	dut1, err := dut1ForUTC(jd1, jd2)
	if err != nil {
		warnEOPUnavailable(mjd)

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

	switch t.scale { //nolint:exhaustive // UTC is identity; only TAI/TT/TDB/UT1 convert
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
		// Pre-1972 the forward conversion uses the Delta-T polynomial rather
		// than Delta-AT — see [Time.TT] — and this direction has to gate on
		// the same epoch or the two stop being inverses.
		//
		// They did stop. UTC->TT->UTC on a 1600 date returned an instant
		// 85.7 seconds later than it started, which is Delta-T(1600) minus
		// the 32.184 s this branch removed on the way back, with Delta-AT
		// contributing nothing because leap seconds do not exist there. A
		// round trip through a scale conversion is the one thing a time
		// library must not lose, and historical work — eclipse canons,
		// ancient observations — is exactly where it was losing it.
		//
		// Delta-T is a function of the UT year, and this branch starts from
		// TT — so a single evaluation would use a year that is wrong by
		// Delta-T itself. That is minutes in 1600 and hours in antiquity,
		// and against a polynomial with real slope it left the round trip
		// 0.097 s out at AD 33.
		//
		// One correction step removes it: evaluate at TT, subtract, then
		// re-evaluate at the UT year that produces. Delta-T changes slowly
		// enough that the second estimate is exact to well below a
		// microsecond, and the forward direction in [Time.TT] evaluates at
		// the same UT year, so the two now agree by construction rather
		// than by luck.
		yTT, _, _, _, _ := gofaext.JdToDate(t.jd1, t.jd2)
		if yTT < 1972 {
			dt := DeltaT(t.DecimalYear())
			approx := fromPartsPreserveLoc(t, t.jd1, t.jd2-dt/86400.0, UTC)
			dt = DeltaT(approx.DecimalYear())

			return fromPartsPreserveLoc(t, t.jd1, t.jd2-dt/86400.0, UTC)
		}

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

	switch t.scale { //nolint:exhaustive // only UTC/TT/TDB convert to TAI
	case UTC:
		// TAI = UTC + ΔAT
		y, m, d, fd, _ := gofaext.JdToDate(t.jd1, t.jd2)
		dat, _ := gofaext.Dat(y, m, d, fd)

		return fromPartsPreserveLoc(t, t.jd1, t.jd2+dat/86400.0, TAI)
	case TT:
		// TAI = TT − 32.184s
		return fromPartsPreserveLoc(t, t.jd1, t.jd2-32.184/86400.0, TAI)
	case TDB:
		// TDB → TT → TAI, all arithmetic.
		//
		// This used to route through UTC with UT1, which is wrong for a
		// conversion between two scales that differ from TAI by a constant
		// and a periodic term. Going via UTC drags in Delta-AT — and, before
		// 1972, Delta-T — neither of which is symmetric here, so
		// TAI->TDB->TAI came back 10,170 seconds late in AD 33: the whole of
		// Delta-T at that epoch, applied on the way home and never on the way
		// out. Measured by TestScaleRoundTripMatrix.
		return t.TT().TAI()
	default:
		// UT1 → UTC → TAI. UT1 genuinely has to go this way: DUT1 is defined
		// against UTC, so there is no arithmetic path.
		return t.UTC().TAI()
	}
}

// TT returns a new Time converted to the Terrestrial Time scale.
//
// For modern dates (post-1972), the conversion uses leap seconds from the
// NAIF LSK: TT = UTC + ΔAT + 32.184s. For historical dates (pre-1972),
// where no leap-second data exists, the Espenak & Meeus (2006) ΔT polynomial
// is used automatically: TT = UT + ΔT. This means .TT() and .TDB() produce
// correct results for any epoch from -1999 to +3000 without requiring the
// user to call ApplyDeltaT() explicitly.
func (t Time) TT() Time {
	if t.scale == TT {
		return t
	}

	switch t.scale { //nolint:exhaustive // only UTC/TAI/TDB convert to TT
	case UTC:
		// Leap seconds were only introduced 1972-01-01; before that, TAI-UTC
		// followed pre-1972 rational drift-rate corrections (still nonzero
		// per SOFA's Dat back to 1960, and zero only before 1960) rather
		// than integer leap seconds, so ΔAT is not a usable basis for TT
		// here regardless of what Dat returns. Gate purely on the epoch.
		y, m, d, fd, _ := gofaext.JdToDate(t.jd1, t.jd2)

		if y < 1972 {
			// Historical date: use ΔT polynomial (TT = UT + ΔT)
			dt := DeltaT(t.DecimalYear())
			return fromPartsPreserveLoc(t, t.jd1, t.jd2+dt/86400.0, TT)
		}
		// Modern date: TT = UTC + ΔAT + 32.184s
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
// This conversion requires IERS Earth Orientation Parameters for DUT1,
// which are loaded automatically and lazily on first need: a pre-seeded
// on-disk cache file, then (if [remote.EnableDownloads] was called for
// [remote.IERSFinals2000A]) a network fetch. Returns an error if none of
// that yields data for the given epoch — unlike [Time.EOP] and
// [Time.UTC]'s UT1 branch, this method propagates the failure rather than
// silently degrading to DUT1=0.
func (t Time) UT1() (Time, error) {
	if t.scale == UT1 {
		return t, nil
	}

	utc := t.UTC() // deterministic route to UTC

	dut1, err := dut1ForUTC(utc.jd1, utc.jd2)
	if err != nil {
		return Time{}, fmt.Errorf("astrogo/time: UT1 conversion failed (MJD %.1f): %w",
			mjdFromJDParts(utc.jd1, utc.jd2), err)
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
