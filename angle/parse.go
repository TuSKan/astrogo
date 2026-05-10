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
// The sign applies to the whole angle; individual fields must be non-negative.
// Arcminutes must be in [0, 60) and arcseconds must be in [0, 60).
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
// A leading '-' sign is allowed for hour angles. Minutes must be in [0, 60)
// and seconds in [0, 60).
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

// parseSexagesimal extracts up to three non-negative numeric fields from s
// (which may begin with an optional sign character).
//
// It accepts any sequence of non-digit, non-'.' characters as a field
// separator, making it tolerant of °, ', ", h, m, s, ':', and whitespace.
// Fields missing from the input default to 0.
func parseSexagesimal(s string) (sign float64, fields [3]float64, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, [3]float64{}, ErrEmptyString
	}

	sign = 1

	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		sign = -1
		s = s[1:]
	}

	s = strings.TrimSpace(s)

	nums, err := extractNumericFields(s, 3)
	if err != nil {
		return 0, [3]float64{}, err
	}

	if len(nums) == 0 {
		return 0, [3]float64{}, ErrNoFields
	}

	var f [3]float64
	copy(f[:], nums)

	return sign, f, nil
}

// extractNumericFields scans s and returns up to limit non-negative float64
// values separated by any run of non-digit, non-dot bytes.
func extractNumericFields(s string, limit int) ([]float64, error) {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return (r < '0' || r > '9') && r != '.'
	})

	var result []float64
	for _, token := range parts {
		if len(result) >= limit {
			break
		}

		v, err := strconv.ParseFloat(token, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", ErrParseNumber, token)
		}

		result = append(result, v)
	}

	return result, nil
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
