package openngc

import (
	"testing"
)

// A malformed coordinate must be refused, not turned into a confident one.
//
// Both parsers discarded every ParseFloat error, so the only thing that could
// fail was the wrong number of colons. "XX:30:00" parsed its hours as zero and
// returned a right ascension of 7.5 degrees with a nil error, which meant the
// caller's own "skip this row" branch never ran: a malformed row became a
// target with a wrong position rather than being dropped.
func TestParseRACoordinateValidation(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		in   string
		want float64
	}{
		{"midnight", "00:00:00.00", 0},
		{"one hour", "01:00:00.00", 15},
		{"M31", "00:42:44.30", 10.684583333333333},
		{"just under a full turn", "23:59:59.99", 359.99995833333335},
	} {
		got, err := parseRA(c.in)
		if err != nil {
			t.Errorf("%s: parseRA(%q): %v", c.name, c.in, err)

			continue
		}

		if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s: parseRA(%q) = %.12f, want %.12f", c.name, c.in, got, c.want)
		}
	}

	for _, c := range []struct{ name, in string }{
		{"hours not a number", "XX:30:00"},
		{"minutes not a number", "12:XX:00"},
		{"seconds not a number", "12:30:XX"},
		{"all empty", "::"},
		{"hours empty", ":30:00"},
		{"too few parts", "12:30"},
		{"too many parts", "12:30:00:00"},
		{"hours past a full turn", "25:00:00"},
		{"minutes past sixty", "12:60:00"},
		{"seconds past sixty", "12:30:60"},
		{"negative hours", "-01:30:00"},
		{"not a coordinate at all", "n/a"},
		{"empty", ""},
	} {
		if got, err := parseRA(c.in); err == nil {
			t.Errorf("%s: parseRA(%q) returned %g with no error", c.name, c.in, got)
		}
	}
}

func TestParseDecCoordinateValidation(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		in   string
		want float64
	}{
		{"equator", "+00:00:00.0", 0},
		{"M31", "+41:16:09.0", 41.269166666666667},
		{"south", "-27:30:00.0", -27.5},
		{"just south of the equator", "-00:30:00.0", -0.5},
		{"north pole", "+90:00:00.0", 90},
		{"south pole", "-90:00:00.0", -90},
		{"unsigned reads as north", "41:16:09.0", 41.269166666666667},
	} {
		got, err := parseDec(c.in)
		if err != nil {
			t.Errorf("%s: parseDec(%q): %v", c.name, c.in, err)

			continue
		}

		if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("%s: parseDec(%q) = %.12f, want %.12f", c.name, c.in, got, c.want)
		}
	}

	for _, c := range []struct{ name, in string }{
		{"degrees not a number", "+XX:30:00"},
		{"minutes not a number", "+41:XX:00"},
		{"seconds not a number", "+41:16:XX"},
		{"all empty", "::"},
		{"too few parts", "+41:16"},
		{"past the pole", "+91:00:00"},
		{"past the other pole", "-91:00:00"},
		{"minutes past sixty", "+41:60:00"},
		{"seconds past sixty", "+41:16:60"},
		{"not a coordinate at all", "n/a"},
		{"empty", ""},
	} {
		if got, err := parseDec(c.in); err == nil {
			t.Errorf("%s: parseDec(%q) returned %g with no error", c.name, c.in, got)
		}
	}
}
