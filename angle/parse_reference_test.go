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

// TestParseReadsWhatTheMarkersSay holds both parsers to the unit markers in
// the string (#752). They used to be separators and nothing more, so fields
// were placed by count: "30'" was 30°, "45\"" was 45°, and a right ascension
// given to ParseDMS came back as degrees of the same number, 15 times too
// small. astropy reads a lone minute or second field as arcminutes or
// arcseconds (its `simple : generic MINUTE | generic SECOND` rule) and
// converts an hour-marked string given unit="deg"; the rows below follow it.
func TestParseReadsWhatTheMarkersSay(t *testing.T) {
	t.Parallel()

	cases := []struct {
		parse   func(string) (angle.Angle, error)
		in      string
		wantDeg float64
		wantErr error // nil: wantDeg applies
	}{
		// A lone arcminute or arcsecond field, which used to be degrees.
		{angle.ParseDMS, "30'", 0.5, nil},
		{angle.ParseDMS, "30′", 0.5, nil},
		{angle.ParseDMS, "30 arcmin", 0, angle.ErrSeparator}, // not astropy's marker
		{angle.ParseDMS, `45"`, 45.0 / 3600, nil},
		{angle.ParseDMS, "45″", 45.0 / 3600, nil},
		{angle.ParseDMS, "45s", 45.0 / 3600, nil},
		{angle.ParseDMS, `30'15"`, 30.0/60 + 15.0/3600, nil},

		// The leading field is not held to [0, 60); one below it is.
		{angle.ParseDMS, "90'", 1.5, nil},
		{angle.ParseDMS, `7200"`, 2, nil},
		{angle.ParseDMS, "1°90'", 0, angle.ErrMinuteRange},
		{angle.ParseDMS, `1'90"`, 0, angle.ErrSecondRange},

		// A skipped field: these are arcseconds, not arcminutes.
		{angle.ParseDMS, `12°30"`, 12 + 30.0/3600, nil},
		{angle.ParseDMS, "12°30''", 12 + 30.0/3600, nil},

		// An hour marker makes the angle an hour angle, whichever function
		// reads it, and a degree marker makes it degrees.
		{angle.ParseDMS, "12h30m00s", 187.5, nil},
		{angle.ParseDMS, "12ʰ30ᵐ", 187.5, nil},
		{angle.ParseDMS, "-1h", -15, nil},
		{angle.ParseHMS, `12°30'00"`, 12.5, nil},
		{angle.ParseHMS, "12d30m", 12.5, nil},

		// With no unit named, each function keeps its own.
		{angle.ParseDMS, "12:30", 12.5, nil},
		{angle.ParseHMS, "12:30", 187.5, nil},
		{angle.ParseHMS, "30m", 7.5, nil},
		{angle.ParseHMS, "45s", 45.0 / 3600 * 15, nil},

		// A unit word after unmarked fields names the whole angle, as in
		// astropy's documented "01:02:30.43 degrees".
		{angle.ParseHMS, "01:02:30.43 degrees", 1 + 2.0/60 + 30.43/3600, nil},
		{angle.ParseDMS, "12 30d", 12.5, nil},
		{angle.ParseDMS, "12:30 h", 187.5, nil},

		// Markers out of order, repeated, or naming two units.
		{angle.ParseDMS, "12'30°", 0, angle.ErrMarker},
		{angle.ParseDMS, `12"30'`, 0, angle.ErrMarker},
		{angle.ParseDMS, "12°30°", 0, angle.ErrMarker},
		{angle.ParseDMS, `12" 30`, 0, angle.ErrMarker},
		{angle.ParseDMS, "12d30h", 0, angle.ErrMarker},
		{angle.ParseDMS, "12h 30 deg", 0, angle.ErrMarker},
		{angle.ParseHMS, "12h30m00s deg", 0, angle.ErrSeparator},
	}

	for _, c := range cases {
		got, err := c.parse(c.in)

		if c.wantErr != nil {
			if !errors.Is(err, c.wantErr) {
				t.Errorf("parse(%+q) = %.9f deg, %v; want %v", c.in, got.Degrees(), err, c.wantErr)
			}

			continue
		}

		if err != nil {
			t.Errorf("parse(%+q): %v; want %.9f deg", c.in, err, c.wantDeg)

			continue
		}

		if math.Abs(got.Degrees()-c.wantDeg) > 1e-12 {
			t.Errorf("parse(%+q) = %.12f deg, want %.12f deg", c.in, got.Degrees(), c.wantDeg)
		}
	}
}

// TestFormattedAnglesParseBack holds the marker rules to the strings this
// package prints: DMSString and HMSString write all three fields, each
// marked, and must come back as the angle they were made from.
func TestFormattedAnglesParseBack(t *testing.T) {
	t.Parallel()

	for _, deg := range []float64{0, 0.5, -0.5, 12.582, -89.999, 179.25} {
		a := angle.Deg(deg)

		dms, err := angle.ParseDMS(a.DMSString(4))
		if err != nil || math.Abs(dms.Degrees()-deg) > 1e-7 {
			t.Errorf("ParseDMS(%q) = %.9f, %v; want %.9f", a.DMSString(4), dms.Degrees(), err, deg)
		}

		if deg < 0 {
			continue
		}

		hms, err := angle.ParseHMS(a.HMSString(4))
		if err != nil || math.Abs(hms.Degrees()-deg) > 1e-6 {
			t.Errorf("ParseHMS(%q) = %.9f, %v; want %.9f", a.HMSString(4), hms.Degrees(), err, deg)
		}
	}
}
