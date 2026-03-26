package angle

import (
	"fmt"
	"strconv"
	"strings"
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
	sign, fields, err := parseSexagesimal(s)
	if err != nil {
		return 0, fmt.Errorf("ParseDMS %q: %w", s, err)
	}
	if err := validateMinSec(fields[1], fields[2]); err != nil {
		return 0, fmt.Errorf("ParseDMS %q: %w", s, err)
	}
	deg := fields[0] + fields[1]/60 + fields[2]/3600
	return Deg(sign * deg), nil
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
	sign, fields, err := parseSexagesimal(s)
	if err != nil {
		return 0, fmt.Errorf("ParseHMS %q: %w", s, err)
	}
	if err := validateMinSec(fields[1], fields[2]); err != nil {
		return 0, fmt.Errorf("ParseHMS %q: %w", s, err)
	}
	hours := fields[0] + fields[1]/60 + fields[2]/3600
	return Hour(sign * hours), nil
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
		return 0, [3]float64{}, fmt.Errorf("empty string")
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
		return 0, [3]float64{}, fmt.Errorf("no numeric fields found")
	}

	var f [3]float64
	copy(f[:], nums)
	return sign, f, nil
}

// extractNumericFields scans s and returns up to max non-negative float64
// values separated by any run of non-digit, non-dot bytes.
func extractNumericFields(s string, max int) ([]float64, error) {
	result := make([]float64, 0, max)
	i := 0
	for i < len(s) && len(result) < max {
		// Skip non-numeric characters
		for i < len(s) && !isDigitByte(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		for i < len(s) && (isDigitByte(s[i]) || s[i] == '.') {
			i++
		}
		token := s[start:i]
		v, err := strconv.ParseFloat(token, 64)
		if err != nil {
			return nil, fmt.Errorf("cannot parse %q as number", token)
		}
		result = append(result, v)
	}
	return result, nil
}

// validateMinSec checks that minutes and seconds are in [0, 60).
func validateMinSec(minutes, secs float64) error {
	if minutes < 0 || minutes >= 60 {
		return fmt.Errorf("minutes %.4g out of range [0, 60)", minutes)
	}
	if secs < 0 || secs >= 60 {
		return fmt.Errorf("seconds %.4g out of range [0, 60)", secs)
	}
	return nil
}

// isDigitByte reports whether b is an ASCII decimal digit.
func isDigitByte(b byte) bool { return b >= '0' && b <= '9' }
