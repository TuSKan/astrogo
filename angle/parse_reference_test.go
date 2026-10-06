package angle_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// TestParseDMSAgreesWithAstropy holds ParseDMS to astropy 8.0.1's Angle
// parser (`Angle(s, unit="deg")`) on the strings where the two used to part
// company (#538).
//
// The old parser took any run of characters other than a digit or '.' as a
// field separator, so it never refused anything: it skipped it. A U+2212
// minus, which typeset sources print in every negative declination, was
// skipped like a colon, and "−12:30:00" came back +12.5° — the other
// hemisphere, with no error. astropy reads it as −12.5°.
//
// Every row's want is what astropy returned for the same string, with one
// deliberate exception marked below.
func TestParseDMSAgreesWithAstropy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in      string
		wantDeg float64
		wantErr error // nil: wantDeg applies
	}{
		// The sign. U+2212 is spelled as an escape so it cannot be mistaken
		// for the ASCII hyphen-minus beside it.
		{"\u221212:30:00", -12.5, nil},
		{"\u221200:30:00", -0.5, nil},
		{"-00:30:00", -0.5, nil},
		{"- 00 30 00", -0.5, nil},
		{"+12:30:00", 12.5, nil},

		// Unit markers astropy accepts, ASCII and typographic.
		{"+12\u00b034\u203256.78\u2033", 12 + 34.0/60 + 56.78/3600, nil},
		{"12deg30min", 12.5, nil},
		{"12d30m00s", 12.5, nil},
		{"12 30 00.5", 12 + 30.0/60 + 0.5/3600, nil},

		// What astropy refuses, and the old parser read as a positive angle.
		{"\u201312:30:00", 0, angle.ErrSeparator}, // en dash: not a minus
		{"12:-30:00", 0, angle.ErrSeparator},      // a sign inside the angle
		{"S12:30:00", 0, angle.ErrSeparator},
		{"1e1", 0, angle.ErrSeparator}, // was 1° 1′
		{"12:30:00:15", 0, angle.ErrTooManyFields},

		// The exception. astropy reads a trailing hemisphere letter as a sign
		// and gets −12.5°. Accepting one would be a feature; refusing it is
		// the fix, since the old parser returned +12.5°.
		{"12:30:00 S", 0, angle.ErrSeparator},
		{"12d30m00s S", 0, angle.ErrSeparator},
	}

	for _, c := range cases {
		got, err := angle.ParseDMS(c.in)

		if c.wantErr != nil {
			if !errors.Is(err, c.wantErr) {
				t.Errorf("ParseDMS(%+q) = %.6f deg, %v; want %v", c.in, got.Degrees(), err, c.wantErr)
			}

			continue
		}

		if err != nil {
			t.Errorf("ParseDMS(%+q): %v; want %.6f deg", c.in, err, c.wantDeg)

			continue
		}

		if math.Abs(got.Degrees()-c.wantDeg) > 1e-12 {
			t.Errorf("ParseDMS(%+q) = %.9f deg, want %.9f deg", c.in, got.Degrees(), c.wantDeg)
		}
	}
}

// TestParseHMSTakesTheSameSigns covers the hour form, where a negative value
// is an hour angle: "−06h" east of the meridian must not become six hours
// west. The superscript markers are how typeset right ascensions are printed.
func TestParseHMSTakesTheSameSigns(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in        string
		wantHours float64
		wantErr   error
	}{
		{"\u221206h00m00s", -6, nil},
		{"12\u02b030\u1d5000\u02e2", 12.5, nil},
		{"12h30m00s", 12.5, nil},
		{"12:30:00 W", 0, angle.ErrSeparator},
		{"06h-30m", 0, angle.ErrSeparator},
	}

	for _, c := range cases {
		got, err := angle.ParseHMS(c.in)

		if c.wantErr != nil {
			if !errors.Is(err, c.wantErr) {
				t.Errorf("ParseHMS(%+q) = %.6fh, %v; want %v", c.in, got.Hours(), err, c.wantErr)
			}

			continue
		}

		if err != nil {
			t.Errorf("ParseHMS(%+q): %v; want %.6fh", c.in, err, c.wantHours)

			continue
		}

		if math.Abs(got.Hours()-c.wantHours) > 1e-12 {
			t.Errorf("ParseHMS(%+q) = %.9fh, want %.9fh", c.in, got.Hours(), c.wantHours)
		}
	}
}

// TestParseKeepsTheFormatsItAlwaysTook is the other side of narrowing the
// separators: every form the package documents or prints must still parse,
// including the degree substitutes a keyboard without a degree key produces.
func TestParseKeepsTheFormatsItAlwaysTook(t *testing.T) {
	t.Parallel()

	for _, in := range []string{
		`+12°34'56.78"`, "-12:34:56.78", "30", "30:15", "30°",
		"12\u00b030''", "12\u00ba30'", "12\u02da30'", "12\u00b0 30\u2032 00\u2033",
		angle.Deg(-23.456789).DMSString(4),
	} {
		if _, err := angle.ParseDMS(in); err != nil {
			t.Errorf("ParseDMS(%+q): %v", in, err)
		}
	}

	for _, in := range []string{"12h34m56.78s", "12:34:56.78", "6", "-06h00m00s", angle.Hour(23.99).HMSString(4)} {
		if _, err := angle.ParseHMS(in); err != nil {
			t.Errorf("ParseHMS(%+q): %v", in, err)
		}
	}
}
