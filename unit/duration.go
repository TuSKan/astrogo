package unit

import (
	"fmt"
	"math"
)

// Duration is an elapsed time, stored in seconds.
//
// The third of the named scalars, after [Length] and [Velocity], and the one
// the other two already implied: [Velocity] is declared as [Meter] over
// [Second], so this package has been doing time arithmetic since it was
// written without a type to say so.
//
// # Why not the standard library's time.Duration
//
// Because it cannot hold an astronomical interval. time.Duration is an int64
// nanosecond count, so it saturates just past ±292 years, and astrogo's own
// epochs routinely run past that — [github.com/TuSKan/astrogo/time.ZeroTime]
// is JD 0, in 4713 BC. Time.Sub used to *wrap* there, reporting a negative
// span for a positive interval between year 1 and 2026, which is the bug that
// put a float64 day count beside it in the first place.
//
// So astrogo already had two ways to hold an interval before this type
// existed: a time.Duration for wall-clock work and a bare float64 of days for
// astronomy. This replaces the second, which was the untyped one. The standard
// library's type remains what a timeout, a ticker or a sleep is measured in,
// because those are wall-clock quantities of a size it holds exactly.
//
// # Why seconds
//
// [Second] is declared here with ScaleFactor 1.0 and every other time unit is
// expressed against it, so seconds is what this package already means by 1.0 —
// the same reason [Length] is meters. Days remain one accessor away, and are
// what most of astronomy is written in.
//
// float64 seconds spans what is asked of it: the age of the universe is 4e17 s,
// well inside the range, and at a 26,000-year precession cycle the ulp is
// 0.1 ms.
//
// # What this does not catch
//
// The gap [Length] documents, with a third edge that is this type's own. A
// bare number means different things to the two duration types a Go program
// now has in scope: 5 is five *nanoseconds* to time.Sleep and five *seconds*
// here. Neither compiler complains, because both are untyped constants
// converting to a named numeric type.
//
// Write [Seconds], [Minutes], [Hours], [Days] or [JulianYears] around a
// literal. The constructors are not a convenience; they are the only place the
// unit gets said.
type Duration float64

// Seconds builds a Duration from a value in seconds.
func Seconds(v float64) Duration { return Duration(v) }

// Minutes builds a Duration from a value in minutes.
func Minutes(v float64) Duration { return Duration(v * Minute.ScaleFactor) }

// Hours builds a Duration from a value in hours.
//
// Note that [github.com/TuSKan/astrogo/angle.Angle] also speaks hours, where
// they are 15° of right ascension rather than 3600 seconds. The two are
// unrelated quantities that share a word, which is why each names its own
// package at the call site.
func Hours(v float64) Duration { return Duration(v * Hour.ScaleFactor) }

// Days builds a Duration from a value in days of 86400 seconds.
func Days(v float64) Duration { return Duration(v * Day.ScaleFactor) }

// JulianYears builds a Duration from a value in Julian years of 365.25 days.
func JulianYears(v float64) Duration { return Duration(v * JulianYear.ScaleFactor) }

// Seconds returns the duration in seconds, which is how it is stored.
func (d Duration) Seconds() float64 { return float64(d) }

// Minutes returns the duration in minutes.
func (d Duration) Minutes() float64 { return float64(d) / Minute.ScaleFactor }

// Hours returns the duration in hours.
func (d Duration) Hours() float64 { return float64(d) / Hour.ScaleFactor }

// Days returns the duration in days of 86400 seconds.
func (d Duration) Days() float64 { return float64(d) / Day.ScaleFactor }

// JulianYears returns the duration in Julian years of 365.25 days.
func (d Duration) JulianYears() float64 { return float64(d) / JulianYear.ScaleFactor }

// IsZero reports whether the duration is exactly zero.
func (d Duration) IsZero() bool { return d == 0 }

// Abs returns the duration's magnitude, discarding its direction in time.
func (d Duration) Abs() Duration { return Duration(math.Abs(float64(d))) }

// Quantity returns the duration as a dimensioned [Quantity], for the
// conversion and dimensional-algebra machinery this package provides.
func (d Duration) Quantity() Quantity { return Quantity{Value: float64(d), Unit: Second} }

// DurationFrom converts a [Quantity] to a Duration, or reports why it cannot.
//
// The error is the point: a Quantity carrying a length or a mass is not a
// duration, and this is where that is noticed rather than at whatever the
// number is later divided by.
func DurationFrom(q Quantity) (Duration, error) {
	s, err := q.In(Second)
	if err != nil {
		return 0, fmt.Errorf("unit: not a duration: %w", err)
	}

	return Duration(s.Value), nil
}

// String renders the duration in the unit a reader of that magnitude expects.
//
// The thresholds are where each unit stops being the natural one to discuss an
// interval in:
//
//	< 1 minute      seconds       an exposure, a light-travel time
//	< 1 hour        minutes       a satellite pass, a short exposure series
//	< 1 day         hours         an observing session
//	< 1 Julian year days          a planet's synodic period
//	otherwise       Julian years  a long-period orbit, a precession cycle
//
// It is for humans. Anything parsing a duration back should use the accessors,
// which say which unit they mean in their own name.
func (d Duration) String() string {
	switch s := math.Abs(float64(d)); {
	case s < Minute.ScaleFactor:
		return fmt.Sprintf("%.6g s", d.Seconds())
	case s < Hour.ScaleFactor:
		return fmt.Sprintf("%.6g min", d.Minutes())
	case s < Day.ScaleFactor:
		return fmt.Sprintf("%.6g h", d.Hours())
	case s < JulianYear.ScaleFactor:
		return fmt.Sprintf("%.6g d", d.Days())
	default:
		return fmt.Sprintf("%.6g a", d.JulianYears())
	}
}
