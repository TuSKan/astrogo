package angle

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Sentinel errors for angle parsing.
var (
	ErrEmptyString = errors.New("empty string")
	ErrNoFields    = errors.New("no numeric fields found")
	ErrParseNumber = errors.New("cannot parse as number")
	ErrMinuteRange = errors.New("minutes out of range [0, 60)")
	ErrSecondRange = errors.New("seconds out of range [0, 60)")
	// ErrSeparator reports text the parser does not recognize as a field
	// separator or unit marker: a hemisphere letter, a sign inside the
	// string, a dash that is not a minus, an exponent.
	ErrSeparator = errors.New("unrecognized text in sexagesimal angle")
	// ErrTooManyFields reports more than the three numeric fields a
	// sexagesimal angle has.
	ErrTooManyFields = errors.New("more than three numeric fields")
	// ErrMarker reports a unit marker that contradicts where it stands: an
	// arcminute marker after the arcseconds, two markers for one field, or a
	// degree and an hour marker in one angle.
	ErrMarker = errors.New("unit marker out of place in sexagesimal angle")
)

// ParseDMS parses a sexagesimal angle string in degrees-arcminutes-arcseconds
// format and returns the corresponding Angle.
//
// Accepted formats (separator styles are interchangeable):
//
//	+DD°MM'SS"         (degree, prime, double-prime)
//	-DD°MM'SS.sss"     (with fractional arcseconds)
//	+DD:MM:SS.sss      (colon-separated)
//	DD:MM:SS           (no sign = positive)
//
// Fields beyond the first are optional: "30°" and "30:00" both parse as 30°.
// The sign applies to the whole angle and may be '+', '-' or U+2212 MINUS
// SIGN; individual fields must be non-negative. Arcminutes and arcseconds
// that follow a larger field must be in [0, 60).
//
// Between and after the fields, whitespace, ':' and the degree, hour, minute
// and second markers astropy accepts (°, d, deg, h, ', ′, m, min, ", ″, s, sec
// and their long forms) may appear. A marker says which field the number
// before it is, as astropy reads it: "30'" is half a degree, "45\"" is 45
// arcseconds, and a marker out of order is [ErrMarker]. An hour marker makes
// the string an hour angle, so "12h30m" is 187.5°: degrees are only the
// default for a string that names no unit. Any other text is [ErrSeparator]
// rather than skipped — including a hemisphere letter, since "12:30:00 S"
// read as a positive angle would be in the wrong hemisphere — and a fourth
// numeric field is [ErrTooManyFields].
func ParseDMS(s string) (Angle, error) {
	return parseBaseSexagesimal(s, "ParseDMS", Deg)
}

// ParseHMS parses a sexagesimal angle string in hours-minutes-seconds format
// and returns the corresponding Angle.
//
// Accepted formats:
//
//	HHhMMmSS.sss s    (letter separators, as produced by HMSString)
//	HH:MM:SS.sss      (colon-separated)
//	HH:MM             (seconds optional)
//
// A leading '-' or U+2212 sign is allowed for hour angles. Minutes and
// seconds that follow a larger field must be in [0, 60). Separators and
// markers follow the same rules as [ParseDMS], which also accepts the
// superscript ʰ ᵐ ˢ of a typeset right ascension, with hours as the default
// unit: "30m" is half an hour, and a degree marker makes the string degrees.
func ParseHMS(s string) (Angle, error) {
	return parseBaseSexagesimal(s, "ParseHMS", Hour)
}

// parseBaseSexagesimal handles the shared logic of extracting the sign and
// components, placing them by their markers, and creating the final angle in
// unit, or in the unit the string names.
func parseBaseSexagesimal(s, funcName string, unit func(float64) Angle) (Angle, error) {
	sign, fields, err := parseSexagesimal(s)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", funcName, s, err)
	}

	val, named, err := place(fields)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", funcName, s, err)
	}

	switch named {
	case degreeMark:
		unit = Deg
	case hourMark:
		unit = Hour
	case unmarked, minuteMark, secondMark:
	}

	return unit(sign * val), nil
}

// place puts each field at the position its marker names, 0 for degrees or
// hours, 1 for minutes and 2 for seconds, and returns their sum in the unit of
// position 0, with the degree or hour marker that named that unit, if any.
//
// A field with no marker takes the position after the previous field, the
// first taking 0. Markers used to be separators and nothing more, so fields
// were placed by count however they were marked: "30'" was thirty degrees,
// "45\"" forty-five, and "12h30m00s" given to ParseDMS was 12.5 degrees
// rather than 187.5 (#752). astropy reads every one of those by its markers.
//
// A degree or hour marker after the last field, with no marker before it,
// names the unit of the whole angle instead of placing that field:
// "01:02:30.43 degrees", the form astropy's own documentation gives.
//
// Only a field below another is held to [0, 60): a lone "90'" is 1.5
// degrees, but "1°90'" is refused.
func place(fields []field) (val float64, named marker, err error) {
	next := 0 // the lowest position the next field may take

	for i, f := range fields {
		pos := next

		switch f.mark {
		case unmarked:
		case degreeMark, hourMark:
			if named != unmarked && named != f.mark {
				return 0, unmarked, fmt.Errorf("%w: %q in an angle already in other units", ErrMarker, f.text)
			}

			named = f.mark

			if i == 0 || i < len(fields)-1 || !unmarkedBefore(fields, i) {
				pos = 0
			}
		case minuteMark:
			pos = 1
		case secondMark:
			pos = 2
		}

		if pos < next || pos > 2 {
			return 0, unmarked, fmt.Errorf("%w: field %d", ErrMarker, i+1)
		}

		if i > 0 {
			if err := validateBelow(pos, f.value); err != nil {
				return 0, unmarked, err
			}
		}

		val += f.value / positionScale[pos]
		next = pos + 1
	}

	return val, named, nil
}

// positionScale divides a field at each position into the unit of position 0.
var positionScale = [3]float64{1, 60, 3600}

// unmarkedBefore reports whether every field before fields[i] is unmarked.
func unmarkedBefore(fields []field, i int) bool {
	for _, f := range fields[:i] {
		if f.mark != unmarked {
			return false
		}
	}

	return true
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// parseSexagesimal extracts up to three non-negative numeric fields from s,
// which may begin with a sign: '+', '-', or U+2212 MINUS SIGN, the character
// typeset sources (journals, Wikipedia, SIMBAD's HTML) print in a negative
// declination.
//
// Between and after the fields only the separators [markerOf] accepts may
// appear.
//
// It used to take any run of characters other than a digit or '.' as a
// separator, so anything it did not understand was skipped rather than
// refused: a U+2212 minus, a trailing "S" for the southern hemisphere, a
// minus inside the string. Each of those came back as a positive angle with
// no error — "−12:30:00" parsed as +12.5°, which is the opposite hemisphere
// (#538).
func parseSexagesimal(s string) (sign float64, fields []field, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil, ErrEmptyString
	}

	sign = 1

	switch {
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	case strings.HasPrefix(s, "-"):
		sign = -1
		s = s[1:]
	case strings.HasPrefix(s, minusSign):
		sign = -1
		s = s[len(minusSign):]
	}

	s = strings.TrimSpace(s)

	fields, err = extractNumericFields(s)
	if err != nil {
		return 0, nil, err
	}

	return sign, fields, nil
}

// field is one number of a sexagesimal angle and the marker after it.
type field struct {
	value float64
	mark  marker
	text  string // the marker as written, for an error
}

// marker is what the text after a field says about it.
type marker int

const (
	unmarked   marker = iota // whitespace or ':': the field after the previous one
	degreeMark               // the degrees, and the angle is in degrees
	hourMark                 // the hours, and the angle is in hours
	minuteMark               // the minutes
	secondMark               // the seconds
)

// extractNumericFields splits s into alternating runs of number (digits and
// '.') and separator, and returns the numbers, each with the marker after it.
// s must start with a number; every separator run must satisfy markerOf.
func extractNumericFields(s string) ([]field, error) {
	var (
		result []field
		lead   string
	)

	for i := 0; i < len(s); {
		j := i
		for j < len(s) && isNumeric(s[j]) {
			j++
		}

		if j == i {
			// A separator run, which may hold multi-byte runes: every byte
			// of one is non-numeric, so it is never split.
			for j < len(s) && !isNumeric(s[j]) {
				j++
			}

			text := strings.TrimSpace(s[i:j])
			if len(result) == 0 {
				lead = text
			} else {
				mark, ok := markerOf(text)
				if !ok {
					return nil, fmt.Errorf("%w: %q", ErrSeparator, text)
				}

				result[len(result)-1].mark = mark
				result[len(result)-1].text = text
			}

			i = j

			continue
		}

		if lead != "" {
			return nil, fmt.Errorf("%w: %q", ErrSeparator, lead)
		}

		if len(result) == 3 {
			return nil, ErrTooManyFields
		}

		v, err := strconv.ParseFloat(s[i:j], 64)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", ErrParseNumber, s[i:j])
		}

		result = append(result, field{value: v})
		i = j
	}

	if len(result) == 0 {
		return nil, ErrNoFields
	}

	return result, nil
}

// minusSign is U+2212 MINUS SIGN, spelled as an escape because in source it
// is indistinguishable from the ASCII hyphen-minus beside it.
const minusSign = "\u2212"

func isNumeric(c byte) bool { return (c >= '0' && c <= '9') || c == '.' }

// markerOf reports what text (already trimmed of space) says about the field
// before it, and whether it may stand between or after the numeric fields at
// all.
//
// The set is astropy's angle grammar — its degree, hour, minute and second
// markers, short and long — plus the colon, two apostrophes for arcseconds,
// and the masculine ordinal 'º' and ring '˚' that keyboards without a degree
// key produce in its place. Empty is whitespace alone. Matching is
// case-sensitive, so a hemisphere "S" or "W" is refused rather than taken for
// seconds: it would mean a negative angle, and reading it as positive is the
// defect this set exists to prevent.
func markerOf(text string) (marker, bool) {
	switch text {
	case "", ":":
		return unmarked, true
	case "°", "º", "˚", "d", "deg", "degree", "degrees":
		return degreeMark, true
	case "h", "hr", "hour", "hours", "ʰ":
		return hourMark, true
	case "m", "min", "minute", "minutes", "'", "′", "ᵐ":
		return minuteMark, true
	case "s", "sec", "second", "seconds", "\"", "″", "''", "ˢ":
		return secondMark, true
	}

	return unmarked, false
}

// validateBelow checks that a field below a larger one, minutes at position 1
// or seconds at position 2, is in [0, 60).
func validateBelow(pos int, v float64) error {
	if v >= 0 && v < 60 {
		return nil
	}

	if pos == 1 {
		return fmt.Errorf("%w: %.4g", ErrMinuteRange, v)
	}

	return fmt.Errorf("%w: %.4g", ErrSecondRange, v)
}
