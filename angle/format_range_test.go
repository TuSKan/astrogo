package angle_test

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// A NaN or infinite angle has no sexagesimal form. Both formatters converted it
// to int64, which is MinInt64 on amd64 and 0 on arm64, so the same NaN printed
// "+0-9223372036854775808°…" on one and "+00°00'0\"" on the other. Compared
// with String rather than with fixed text, so the test fails on either (#992).
func TestNonFiniteAnglesFormatAsString(t *testing.T) {
	t.Parallel()

	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		a := angle.Rad(v)

		want := a.String()
		// String must show what the value is, or matching it proves nothing.
		if !strings.Contains(want, "NaN") && !strings.Contains(want, "Inf") {
			t.Fatalf("String() of %v = %q shows neither NaN nor Inf", v, want)
		}

		for _, precision := range []int{-1, 0, 2, 400} {
			if got := a.DMSString(precision); got != want {
				t.Errorf("DMSString(%d) of %v = %q, want %q as String gives", precision, v, got, want)
			}

			if got := a.HMSString(precision); got != want {
				t.Errorf("HMSString(%d) of %v = %q, want %q as String gives", precision, v, got, want)
			}
		}
	}
}

// DMSString does not wrap, so its degrees can exceed any int64, and from 2^63
// the conversion overflowed: Deg(1e19) printed MinInt64 digits. A float64 that
// large is a whole number, so the printed degrees must be exactly the angle's,
// and the minutes and seconds zero (#992).
func TestDMSStringPastTheInt64Range(t *testing.T) {
	t.Parallel()

	for _, deg := range []float64{0x1p63, 1e19, -1e19, 1e300} {
		a := angle.Deg(deg)
		got := a.DMSString(2)

		if reason := wellFormedDMS(got); reason != "" {
			t.Errorf("DMSString(2) of %g degrees = %q: %s", deg, got, reason)

			continue
		}

		wantSign := "+"
		if deg < 0 {
			wantSign = "-"
		}

		whole, rest, _ := strings.Cut(got[1:], "°")

		if got[:1] != wantSign || rest != `00'00.00"` {
			t.Errorf("DMSString(2) of %g degrees = %q, want %s<degrees>°00'00.00\"", deg, got, wantSign)
		}

		if strings.Trim(whole, "0123456789") != "" {
			t.Errorf("DMSString(2) of %g degrees = %q: degrees %q are not digits", deg, got, whole)

			continue
		}

		printed, err := strconv.ParseFloat(whole, 64)
		if want := math.Abs(a.Degrees()); err != nil || printed != want {
			t.Errorf("DMSString(2) of %g degrees prints %s degrees, want exactly %.0f", deg, whole, want)
		}
	}
}

// Once the seconds are printed to 17 significant digits they name one float64,
// so a higher precision may only add digits of that same value, under the same
// degrees and minutes. That is from precision 15 for seconds of 10 or more,
// and 16 for seconds from 1 to 10.
//
// Rounding by scaling with 10^precision broke this from the first of those.
// Past 2^53 the scaled value is already whole, and dividing back only adds
// error: the seconds of Deg(59.995/3600) are 59.9950000000000117 at precision
// 16 but printed 59.995000000000005 at 15. From 307 the scaled value overflowed
// and carried a minute, printing 1°30'30" as 1°31'00", and 10^309 is +Inf,
// which printed NaN seconds for every angle (#992).
//
// The digits are never pinned: arm64 fuses the seconds arithmetic, so the
// value differs there in its last bits.
func TestSecondsKeepTheirValueAtAnyPrecision(t *testing.T) {
	t.Parallel()

	// Seconds of zero, just under 10 (a leading zero), mid-range, and just
	// under 60 (one step from a carry); 59.995 drifted at precision 15.
	dms := []angle.Angle{
		angle.Deg(0),
		angle.Deg(9.99999999 / 3600),
		angle.Deg(59.995 / 3600),
		angle.Deg(1 + 30.0/60 + 30.0/3600),
		angle.Deg(1 + 30.0/60 + 1.0/3600),
		angle.Deg(-(12 + 34.0/60 + 56.789/3600)),
		angle.Deg(1 + 59.99999999/3600),
	}

	for _, a := range dms {
		checkSecondsKeepTheirValue(t, "DMSString", a.DMSString, splitDMS, wellFormedDMS)
	}

	hms := []angle.Angle{
		angle.Hour(0),
		angle.Hour(9.99999999 / 3600),
		angle.Hour(59.995 / 3600),
		angle.Hour(2 + 30.0/60 + 15.5/3600),
		angle.Hour(23 + 59.0/60 + 59.99999999/3600),
	}

	for _, a := range hms {
		checkSecondsKeepTheirValue(t, "HMSString", a.HMSString, splitHMS, wellFormedHMS)
	}
}

// checkSecondsKeepTheirValue holds format to TestSecondsKeepTheirValueAtAnyPrecision's
// claim, from the first precision at which its seconds carry 17 significant
// digits up to 400, and at 1000.
func checkSecondsKeepTheirValue(
	t *testing.T, name string, format func(int) string,
	split func(string) sexagesimal, wellFormed func(string) string,
) {
	t.Helper()

	// At 17 decimal places any seconds of 1 or more have 17 significant digits,
	// enough to tell whether the value is under 10.
	seventeen, err := strconv.ParseFloat(split(format(17)).seconds, 64)
	if err != nil {
		t.Fatalf("%s(17) = %q: seconds do not parse", name, format(17))
	}

	if seventeen != 0 && seventeen < 1 {
		t.Fatalf("%s(17) = %q: seconds under 1 need more than 17 places, pick another angle", name, format(17))
	}

	reference := 15
	if seventeen != 0 && seventeen < 10 {
		reference = 16
	}

	ref := split(format(reference))

	precisions := []int{1000}
	for p := reference; p <= 400; p++ {
		precisions = append(precisions, p)
	}

	for _, p := range precisions {
		got := format(p)
		if reason := wellFormed(got); reason != "" {
			t.Errorf("%s(%d) = %.60q: %s", name, p, got, reason)

			continue
		}

		if reason := sameValue(ref, split(got)); reason != "" {
			t.Errorf("%s(%d) = %.60q: %s than at precision %d, %q", name, p, got, reason, reference, ref)
		}
	}
}

// sexagesimal is a formatted angle's leading field with its sign, its minutes,
// and its seconds.
type sexagesimal struct {
	lead, minutes, seconds string
}

// splitDMS splits a string wellFormedDMS accepts.
func splitDMS(s string) sexagesimal {
	lead, rest, _ := strings.Cut(s, "°")
	minutes, seconds, _ := strings.Cut(rest, "'")

	return sexagesimal{lead, minutes, strings.TrimSuffix(seconds, `"`)}
}

// splitHMS splits a string wellFormedHMS accepts.
func splitHMS(s string) sexagesimal {
	lead, rest, _ := strings.Cut(s, "h")
	minutes, seconds, _ := strings.Cut(rest, "m")

	return sexagesimal{lead, minutes, strings.TrimSuffix(seconds, "s")}
}

// sameValue returns how got differs from ref, or "" if both name the same
// degrees or hours, minutes and seconds.
func sameValue(ref, got sexagesimal) string {
	if got.lead != ref.lead || got.minutes != ref.minutes {
		return "other degrees, hours or minutes"
	}

	r, err := strconv.ParseFloat(ref.seconds, 64)
	if err != nil {
		return "reference seconds do not parse"
	}

	g, err := strconv.ParseFloat(got.seconds, 64)
	if err != nil || g != r {
		return "other seconds"
	}

	return ""
}
