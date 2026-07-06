package lsk

import (
	"errors"
	"testing"
)

// TestParseSpiceDate_MalformedYearAndDay is a regression test: parseSpiceDate
// used to discard strconv.Atoi errors on the year and day fields, silently
// producing year=0/day=0 (a JD in the deep past) instead of rejecting the
// entry. The caller in NewReader only skips an entry when parseSpiceDate
// returns a non-nil error, so a malformed field must surface as an error
// here, not as a bogus zero-value date.
func TestParseSpiceDate_MalformedYearAndDay(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"malformed year", "@YYYY-JAN-01"},
		{"malformed day", "@2016-JAN-XX"},
		{"malformed year, no day", "@YYYY-JAN"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseSpiceDate(c.in)
			if err == nil {
				t.Fatalf("parseSpiceDate(%q): expected error, got nil", c.in)
			}

			if !errors.Is(err, ErrInvalidDate) {
				t.Errorf("parseSpiceDate(%q): error %v does not wrap ErrInvalidDate", c.in, err)
			}
		})
	}
}

// TestParseSpiceDate_WellFormed confirms the happy path still works after the
// error-discard fix (day defaults to 1 when omitted).
func TestParseSpiceDate_WellFormed(t *testing.T) {
	jdWithDay, err := parseSpiceDate("@2016-JAN-01")
	if err != nil {
		t.Fatalf("parseSpiceDate(@2016-JAN-01): unexpected error: %v", err)
	}

	jdNoDay, err := parseSpiceDate("@2016-JAN")
	if err != nil {
		t.Fatalf("parseSpiceDate(@2016-JAN): unexpected error: %v", err)
	}

	if jdWithDay != jdNoDay {
		t.Errorf("parseSpiceDate(@2016-JAN-01)=%v should equal parseSpiceDate(@2016-JAN)=%v (day defaults to 1)", jdWithDay, jdNoDay)
	}
}
