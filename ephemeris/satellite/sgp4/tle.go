package sgp4

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/time"
)

// LineLength is the fixed width of a TLE line, check digit included.
//
// Exported because it is what a caller has to slice to when a feed appends
// extra fields — Vallado's own verification file writes three propagation
// parameters past column 69 on every line 2.
const LineLength = 69

// Column spans, 0-based and half-open, exactly as the format fixes them.
//
// Written as explicit start/end pairs rather than chaining each field's end to
// the next field's start, because the fields are NOT adjacent — single blank
// columns separate several of them, and treating a separator as part of its
// neighbour silently widens the field. That is not a theoretical hazard: the
// first draft of this file did exactly that and handed an extra column to both
// assumed-exponent fields.
//
// An off-by-one here mostly does not fail. It reads the neighbouring field,
// which is also a number.
type span struct{ start, end int }

var (
	// Line 1.
	spanSatNum1    = span{2, 7}
	spanDesignator = span{9, 17}
	spanEpochYear  = span{18, 20}
	spanEpochDay   = span{20, 32}
	spanNDot       = span{33, 43}
	spanNDDot      = span{44, 52}
	spanBStar      = span{53, 61}
	spanElementSet = span{64, 68}

	// Line 2.
	spanSatNum2     = span{2, 7}
	spanInclination = span{8, 16}
	spanRAAN        = span{17, 25}
	spanEcc         = span{26, 33}
	spanArgPerigee  = span{34, 42}
	spanMeanAnomaly = span{43, 51}
	spanMeanMotion  = span{52, 63}
	spanRevNumber   = span{63, 68}
)

// Single columns, which have no end to get wrong.
const (
	colLineNumber = 0
	colClass      = 7
	colChecksum   = 68
)

// ParseTLE reads a two-line element set.
//
// It does NOT verify the check digits — see [VerifyTLEChecksums] and
// [ErrChecksum] for why that is a separate question — but it does validate the
// elements themselves, so a returned [Elements] has already passed
// [Elements.Validate].
//
// Lines longer than [LineLength] are accepted and the excess ignored, because
// real sources append to them: Vallado's verification file carries three extra
// fields on every line 2, and trailing whitespace and CR are everywhere.
//
// Use [ParseTLEName] when the source also supplies a title line.
//
// # On being strict
//
// Every field is parsed and every failure is reported, with the field named and
// the offending text quoted. That sounds unremarkable and it is the entire
// reason this package exists in the form it does: the implementation astrogo
// used before this one reacted to a malformed element set by calling
// [log.Fatal], or by walking off the end of a twelve-element array, or by
// returning a zero value with a nil error. A library cannot exit the process
// its caller is running.
func ParseTLE(line1, line2 string) (Elements, error) {
	return ParseTLEName("", line1, line2)
}

// ParseTLEName is [ParseTLE] for a source that also carries a title line.
//
// name is trimmed and stored in [Elements.Name]; it is not validated, because
// a title line is free text that catalogues pad, truncate and occasionally
// leave blank, and nothing in the model reads it.
func ParseTLEName(name, line1, line2 string) (Elements, error) {
	l1, l2, err := trimToLines(line1, line2)
	if err != nil {
		return Elements{}, err
	}

	el := Elements{Name: strings.TrimSpace(name)}

	if el.NORAD, err = parseSatelliteNumbers(l1, l2); err != nil {
		return Elements{}, err
	}

	el.Classification = l1[colClass]
	el.Designator = strings.TrimSpace(field(l1, spanDesignator))

	if el.Epoch, err = parseEpoch(l1); err != nil {
		return Elements{}, err
	}

	// Line 1's numeric fields. The two assumed-exponent ones are read by
	// parseExponential; the rest are ordinary decimals with a possible sign.
	if el.MeanMotionDot, err = parseFloatField(l1, spanNDot, "first derivative of mean motion"); err != nil {
		return Elements{}, err
	}

	if el.MeanMotionDDot, err = parseExponential(l1, spanNDDot, "second derivative of mean motion"); err != nil {
		return Elements{}, err
	}

	if el.BStar, err = parseExponential(l1, spanBStar, "B* drag term"); err != nil {
		return Elements{}, err
	}

	if el.ElementSet, err = parseIntField(l1, spanElementSet, "element set number"); err != nil {
		return Elements{}, err
	}

	// Line 2.
	incl, err := parseAngleField(l2, spanInclination, "inclination")
	if err != nil {
		return Elements{}, err
	}

	el.Inclination = incl

	if el.RAAN, err = parseAngleField(l2, spanRAAN, "right ascension of the ascending node"); err != nil {
		return Elements{}, err
	}

	if el.Eccentricity, err = parseEccentricity(l2); err != nil {
		return Elements{}, err
	}

	if el.ArgPerigee, err = parseAngleField(l2, spanArgPerigee, "argument of perigee"); err != nil {
		return Elements{}, err
	}

	if el.MeanAnomaly, err = parseAngleField(l2, spanMeanAnomaly, "mean anomaly"); err != nil {
		return Elements{}, err
	}

	if el.MeanMotion, err = parseFloatField(l2, spanMeanMotion, "mean motion"); err != nil {
		return Elements{}, err
	}

	if el.RevAtEpoch, err = parseIntField(l2, spanRevNumber, "revolution number at epoch"); err != nil {
		return Elements{}, err
	}

	if err := el.Validate(); err != nil {
		return Elements{}, err
	}

	return el, nil
}

// trimToLines checks the two lines are structurally a TLE and returns them cut
// to exactly [LineLength].
func trimToLines(line1, line2 string) (l1, l2 string, err error) {
	for i, raw := range [2]string{line1, line2} {
		want := byte('1' + i)

		trimmed := strings.TrimRight(raw, " \t\r\n")
		if len(trimmed) < LineLength {
			return "", "", fmt.Errorf("%w: line %d is %d characters, want at least %d",
				ErrMalformedTLE, i+1, len(trimmed), LineLength)
		}

		trimmed = trimmed[:LineLength]

		if trimmed[colLineNumber] != want {
			return "", "", fmt.Errorf("%w: line %d starts with %q, want %q",
				ErrMalformedTLE, i+1, trimmed[colLineNumber], want)
		}

		if i == 0 {
			l1 = trimmed
		} else {
			l2 = trimmed
		}
	}

	return l1, l2, nil
}

// parseSatelliteNumbers reads the catalogue number from both lines and refuses
// two lines that describe different objects.
//
// Worth checking rather than reading one and trusting it: a feed that
// interleaves objects, or a copy-paste that takes line 1 from one set and line
// 2 from another, produces a perfectly parseable element set for a satellite
// that does not exist. The check digits do not catch it — each line is
// individually intact.
func parseSatelliteNumbers(l1, l2 string) (int, error) {
	n1, err := parseIntField(l1, spanSatNum1, "satellite number on line 1")
	if err != nil {
		return 0, err
	}

	n2, err := parseIntField(l2, spanSatNum2, "satellite number on line 2")
	if err != nil {
		return 0, err
	}

	if n1 != n2 {
		return 0, fmt.Errorf("%w: line 1 is satellite %d and line 2 is satellite %d",
			ErrMalformedTLE, n1, n2)
	}

	return n1, nil
}

// parseEpoch reads the two-digit year and fractional day of year.
//
// # The day of year is bounded here, and the reason is a crash
//
// A day of year past the end of its year is a number, parses fine, and is
// meaningless. The implementation astrogo used before this one converted it by
// subtracting month lengths in a loop guarded against an index of 22 over a
// twelve-element array, so day 400 walked off the end and panicked — found by a
// fuzzer in about two seconds, on an element set that was 69 columns,
// checksum-valid and numeric in every field.
//
// Nothing here indexes a month table, so nothing here would panic. The bound
// stays anyway, because a day of year outside its own year is not an epoch and
// silently producing a date in the following March is worse than saying so.
func parseEpoch(l1 string) (time.Time, error) {
	yy, err := parseIntField(l1, spanEpochYear, "epoch year")
	if err != nil {
		return time.Time{}, err
	}

	// The TLE format's own pivot, which expires in 2057.
	year := 2000 + yy
	if yy >= 57 {
		year = 1900 + yy
	}

	doy, err := parseFloatField(l1, spanEpochDay, "epoch day")
	if err != nil {
		return time.Time{}, err
	}

	daysInYear := 365.0
	if isLeap(year) {
		daysInYear = 366.0
	}

	// Day 1.0 is midnight on January 1st, so the valid half-open range is
	// [1, daysInYear+1).
	if doy < 1.0 || doy >= daysInYear+1.0 {
		return time.Time{}, fmt.Errorf(
			"%w: epoch day %g is outside [1, %g) for year %d",
			ErrMalformedTLE, doy, daysInYear+1.0, year)
	}

	jan1 := time.Date(year, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	// AddDays, not Add(Duration): Duration quantises to nanoseconds via a
	// float64 product that exceeds 2^53 ns for a late-year epoch, and AddDays
	// advances the calendar label — which is what a day-of-year is.
	return jan1.AddDays(doy - 1.0), nil
}

// isLeap is the Gregorian rule.
//
// Within the two-digit year's own window — 1957 to 2056 — it agrees with the
// naive year%4 test, since 1900 and 2100 both fall outside. Spelled correctly
// anyway: the window is a property of the format, not of arithmetic, and a
// reader should not have to verify that coincidence to trust the line.
func isLeap(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// parseEccentricity reads the seven digits after an assumed decimal point.
func parseEccentricity(l2 string) (float64, error) {
	raw := field(l2, spanEcc)

	v, err := strconv.ParseFloat("."+strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("%w: eccentricity is %q, which is not a number (a TLE stores it "+
			"with an assumed leading decimal point)", ErrMalformedTLE, raw)
	}

	return v, nil
}

// parseExponential reads a TLE's assumed-decimal, assumed-exponent field:
// eight columns holding a sign, five mantissa digits and a signed exponent, so
// " 12345-3" is 0.12345e-3 and "-11606-4" is -0.11606e-4.
//
// The format exists to fit a number with a wide dynamic range into eight
// columns and it has no decimal point, no 'e', and no requirement that the
// signs be written at all — a blank is a plus. Assembling the pieces into
// something strconv can read is the only sane way to parse it, and getting the
// slices wrong yields another perfectly good number.
func parseExponential(line string, at span, name string) (float64, error) {
	raw := field(line, at)

	const (
		mantissaLen = 5
		fieldLen    = 8
	)

	if len(raw) != fieldLen {
		return 0, fmt.Errorf("%w: %s field is %d columns, want %d", ErrMalformedTLE, name, len(raw), fieldLen)
	}

	sign := ""

	switch raw[0] {
	case '-':
		sign = "-"
	case '+', ' ', '0':
		// A blank sign column is a plus. '0' appears in feeds that zero-pad.
	default:
		return 0, fmt.Errorf("%w: %s begins with %q, want a sign or a blank",
			ErrMalformedTLE, name, raw[0])
	}

	mantissa := strings.TrimSpace(raw[1 : 1+mantissaLen])
	if mantissa == "" {
		mantissa = "0"
	}

	exponent := strings.TrimSpace(raw[1+mantissaLen:])
	if exponent == "" || exponent == "+" || exponent == "-" {
		exponent = "0"
	}

	assembled := sign + "." + mantissa + "e" + exponent

	v, err := strconv.ParseFloat(assembled, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s is %q, which does not read as a number even as %q",
			ErrMalformedTLE, name, raw, assembled)
	}

	return v, nil
}

// parseFloatField reads an ordinary decimal field.
func parseFloatField(line string, at span, name string) (float64, error) {
	raw := field(line, at)

	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s is %q, which is not a number", ErrMalformedTLE, name, raw)
	}

	return v, nil
}

// parseAngleField reads a decimal field in degrees.
func parseAngleField(line string, at span, name string) (angle.Angle, error) {
	v, err := parseFloatField(line, at, name)
	if err != nil {
		return 0, err
	}

	return angle.Deg(v), nil
}

// parseIntField reads a decimal integer field, treating an all-blank field as
// zero — line 1's element set number and line 2's revolution number are both
// right-aligned in a field wide enough to be entirely empty.
func parseIntField(line string, at span, name string) (int, error) {
	raw := strings.TrimSpace(field(line, at))
	if raw == "" {
		return 0, nil
	}

	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: %s is %q, which is not an integer",
			ErrMalformedTLE, name, field(line, at))
	}

	return v, nil
}

// field returns the columns at names, from a line already cut to LineLength.
func field(line string, at span) string { return line[at.start:at.end] }

// Checksum returns the modulo-10 sum a TLE line's last character records:
// every digit counts for its value and every minus sign for one, with
// everything else — letters, spaces, decimal points and plus signs — counting
// for nothing.
//
// The line's own final character is not included, so this can be compared
// against it directly.
func Checksum(line string) int {
	sum := 0

	for i := 0; i < len(line) && i < colChecksum; i++ {
		switch c := line[i]; {
		case c >= '0' && c <= '9':
			sum += int(c - '0')
		case c == '-':
			sum++
		}
	}

	return sum % 10
}

// VerifyTLEChecksums checks both lines' check digits.
//
// Deliberately separate from [ParseTLE]: see [ErrChecksum]. A caller reading a
// network feed should call both, and does when it goes through
// [github.com/TuSKan/astrogo/ephemeris/satellite]. A caller reading a file
// somebody hand-edited — Vallado's verification suite, say — should call only
// the parser.
func VerifyTLEChecksums(line1, line2 string) error {
	l1, l2, err := trimToLines(line1, line2)
	if err != nil {
		return err
	}

	for i, line := range [2]string{l1, l2} {
		want := int(line[colChecksum] - '0')
		if line[colChecksum] < '0' || line[colChecksum] > '9' {
			return fmt.Errorf("%w: line %d ends with %q, which is not a check digit",
				ErrChecksum, i+1, line[colChecksum])
		}

		if got := Checksum(line); got != want {
			return fmt.Errorf("%w: line %d records %d, computed %d", ErrChecksum, i+1, want, got)
		}
	}

	return nil
}
