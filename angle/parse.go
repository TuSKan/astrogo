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
// SIGN; individual fields must be non-negative. Arcminutes must be in [0, 60)
// and arcseconds must be in [0, 60).
//
// Between and after the fields, whitespace, ':' and the degree, minute and
// second markers astropy accepts (°, d, deg, ', ′, m, min, ", ″, s, sec and
// their long forms) may appear. Any other text is [ErrSeparator] rather than
// skipped — including a hemisphere letter, since "12:30:00 S" read as a
// positive angle would be in the wrong hemisphere — and a fourth numeric
// field is [ErrTooManyFields].
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
// A leading '-' or U+2212 sign is allowed for hour angles. Minutes must be in
// [0, 60) and seconds in [0, 60). Separators follow the same rules as
// [ParseDMS], which also accepts the superscript ʰ ᵐ ˢ of a typeset right
// ascension.
func ParseHMS(s string) (Angle, error) {
	return parseBaseSexagesimal(s, "ParseHMS", Hour)
}

// parseBaseSexagesimal handles the shared logic of extracting the sign and components,
// validating bounds, and creating the final angle.
func parseBaseSexagesimal(s, funcName string, unit func(float64) Angle) (Angle, error) {
	sign, fields, err := parseSexagesimal(s)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", funcName, s, err)
	}

	err = validateMinSec(fields[1], fields[2])
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", funcName, s, err)
	}

	val := fields[0] + fields[1]/60 + fields[2]/3600

	return unit(sign * val), nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// parseSexagesimal extracts up to three non-negative numeric fields from s,
// which may begin with a sign: '+', '-', or U+2212 MINUS SIGN, the character
// typeset sources (journals, Wikipedia, SIMBAD's HTML) print in a negative
// declination.
//
// Between and after the fields only the separators [isSeparator] accepts may
// appear. Fields missing from the input default to 0.
//
// It used to take any run of characters other than a digit or '.' as a
// separator, so anything it did not understand was skipped rather than
// refused: a U+2212 minus, a trailing "S" for the southern hemisphere, a
// minus inside the string. Each of those came back as a positive angle with
// no error — "−12:30:00" parsed as +12.5°, which is the opposite hemisphere
// (#538).
func parseSexagesimal(s string) (sign float64, fields [3]float64, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, [3]float64{}, ErrEmptyString
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

	nums, err := extractNumericFields(s)
	if err != nil {
		return 0, [3]float64{}, err
	}

	copy(fields[:], nums)

	return sign, fields, nil
}

// extractNumericFields splits s into alternating runs of number (digits and
// '.') and separator, and returns the numbers. s must start with a number;
// every separator run must satisfy isSeparator.
func extractNumericFields(s string) ([]float64, error) {
	var (
		result []float64
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
			} else if !isSeparator(text) {
				return nil, fmt.Errorf("%w: %q", ErrSeparator, text)
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

		result = append(result, v)
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

// isSeparator reports whether text (already trimmed of space) may stand
// between or after the numeric fields.
//
// The set is astropy's angle grammar — its degree, hour, minute and second
// markers, short and long — plus the colon, two apostrophes for arcseconds,
// and the masculine ordinal 'º' and ring '˚' that keyboards without a degree
// key produce in its place. Empty is whitespace alone. Matching is
// case-sensitive, so a hemisphere "S" or "W" is refused rather than taken for
// seconds: it would mean a negative angle, and reading it as positive is the
// defect this set exists to prevent.
func isSeparator(text string) bool {
	switch text {
	case "", ":",
		"°", "º", "˚", "d", "deg", "degree", "degrees",
		"h", "hr", "hour", "hours", "ʰ",
		"m", "min", "minute", "minutes", "'", "′", "ᵐ",
		"s", "sec", "second", "seconds", "\"", "″", "''", "ˢ":
		return true
	}

	return false
}

// validateMinSec checks that minutes and seconds are in [0, 60).
func validateMinSec(minutes, secs float64) error {
	if minutes < 0 || minutes >= 60 {
		return fmt.Errorf("%w: %.4g", ErrMinuteRange, minutes)
	}

	if secs < 0 || secs >= 60 {
		return fmt.Errorf("%w: %.4g", ErrSecondRange, secs)
	}

	return nil
}
