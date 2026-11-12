package mpcorb_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/mpcorb"
	"github.com/TuSKan/astrogo/catalog/resolve"
)

// replaceField returns row with [start, end) replaced by text, right-aligned
// in the field as the MPC writes numbers.
func replaceField(t *testing.T, row string, start, end int, text string) string {
	t.Helper()

	if len(text) > end-start {
		t.Fatalf("%q does not fit columns %d-%d", text, start+1, end)
	}

	return row[:start] + strings.Repeat(" ", end-start-len(text)) + text + row[end:]
}

// readOne reads a single row and returns its target or its error.
func readOne(t *testing.T, row string) (resolve.Target, error) {
	t.Helper()

	for tgt, err := range mpcorb.Read(strings.NewReader(row + "\n")) {
		return tgt, err
	}

	t.Fatal("Read yielded nothing")

	return resolve.Target{}, nil
}

// A non-finite number in an MPC row is read as one that does not parse
// (#1080): a required orbital element makes the row malformed, and an optional
// magnitude is absent. strconv.ParseFloat accepts NaN and the infinities, so
// before #1080 they were taken as given.
func TestReadTreatsANonFiniteNumberAsUnparseable(t *testing.T) {
	t.Parallel()

	eros, _, _ := strings.Cut(fixture, "\n")

	// Columns 27-35, the mean anomaly: required.
	for _, bad := range []string{"NaN", "Inf", "-Infinity"} {
		if _, err := readOne(t, replaceField(t, eros, 26, 35, bad)); !errors.Is(err, mpcorb.ErrMalformedRow) {
			t.Errorf("mean anomaly %q: got %v, want ErrMalformedRow", bad, err)
		}
	}

	// Columns 9-13, the absolute magnitude: optional, so absent rather than an
	// error, and the row keeps its orbit.
	tgt, err := readOne(t, replaceField(t, eros, 8, 13, "NaN"))
	if err != nil {
		t.Fatalf("H NaN: %v", err)
	}

	if tgt.HasH {
		t.Errorf("H NaN: HasH is true with H %v", tgt.H)
	}

	if !tgt.HasElements {
		t.Error("H NaN cost the row its orbital elements")
	}
}

// A comet's perihelion day of NaN is refused. It passed both of the day's
// range comparisons, since every comparison with NaN is false, and reached
// int(math.Floor(NaN)).
func TestReadRefusesANonFinitePerihelionDay(t *testing.T) {
	t.Parallel()

	// Columns 23-29, the perihelion day.
	for _, bad := range []string{"NaN", "+Inf"} {
		if _, err := readOne(t, replaceField(t, cometRows[0], 22, 29, bad)); !errors.Is(err, mpcorb.ErrMalformedRow) {
			t.Errorf("perihelion day %q: got %v, want ErrMalformedRow", bad, err)
		}
	}

	// Columns 92-95, the comet's absolute magnitude: optional.
	tgt, err := readOne(t, replaceField(t, cometRows[0], 91, 95, "NaN"))
	if err != nil {
		t.Fatalf("comet H NaN: %v", err)
	}

	if tgt.HasM1 {
		t.Errorf("comet H NaN was kept as M1 %v", tgt.M1)
	}
}
