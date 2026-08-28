package iers_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/time/internal/iers"
)

// finalsLine builds one finals2000A row with the fields at the fixed columns
// the reader slices: MJD at 7:15, polar x at 18:27, polar y at 37:46, UT1-UTC
// at 58:68 and length of day at 79:86.
//
// Passing an empty string for a field leaves those columns blank, which is
// what the bulletin itself does past the end of its data — and, importantly,
// it stays padded to full width there rather than truncating the line, so a
// length check does not catch it.
func finalsLine(mjd, x, y, dut1, lod string) string {
	row := []byte(strings.Repeat(" ", 90))

	place := func(s string, lo, hi int) {
		if s == "" {
			return
		}

		// Right-aligned within the field, as the bulletin writes them.
		copy(row[max(hi-len(s), lo):hi], s)
	}

	place(mjd, 7, 15)
	place(x, 18, 27)
	place(y, 37, 46)
	place(dut1, 58, 68)
	place(lod, 79, 86)

	return string(row)
}

// A row with an MJD and no orientation is the bulletin running out, not an
// epoch at which the pole is centred and UT1 equals UTC.
//
// finals2000A is padded to full width for its whole length, so those rows are
// not short and a length check does not reach them. Parsing them with the
// error discarded stored records of exactly zero at the end of the table —
// fifty of them in the file cached while this was written. Over that window
// Coverage claimed data it did not have, EOP answered instead of returning
// ErrOutOfRange, the one-time "EOP unavailable" warning therefore never fired,
// and the last real day was interpolated into a fabricated zero. A DUT1 of
// zero asserts UT1 = UTC, wrong by up to 0.9 seconds — thirteen and a half
// arcseconds of Earth rotation, applied silently.
func TestBlankOrientationRowsAreNotCoverage(t *testing.T) {
	t.Parallel()

	bulletin := strings.Join([]string{
		finalsLine("60000.00", "0.123456", "0.234567", "-0.0451234", "0.0012"),
		finalsLine("60001.00", "0.123556", "0.234467", "-0.0461234", "0.0013"),
		finalsLine("60002.00", "0.123656", "0.234367", "-0.0471234", ""),
		// The bulletin runs out here: MJD present, everything else blank.
		finalsLine("60003.00", "", "", "", ""),
		finalsLine("60004.00", "", "", "", ""),
	}, "\n")

	table, err := iers.ParseFinals2000A(strings.NewReader(bulletin))
	if err != nil {
		t.Fatalf("ParseFinals2000A: %v", err)
	}

	lo, hi := table.Coverage()
	if lo != 60000 || hi != 60002 {
		t.Errorf("coverage is [%.1f, %.1f], want [60000.0, 60002.0] — the blank rows are not data",
			lo, hi)
	}

	// The last real row still answers, with its real value.
	last, err := table.EOP(60002)
	if err != nil {
		t.Fatalf("EOP at the last real row: %v", err)
	}

	if last.DUT1 == 0 {
		t.Error("the last real row came back with DUT1 of zero")
	}

	// Past it, the answer is a refusal, which is what routes a caller to the
	// zero-EOP degradation and its warning rather than past it.
	for _, mjd := range []float64{60002.5, 60003, 60004, 60005} {
		if _, err := table.EOP(mjd); !errors.Is(err, iers.ErrOutOfRange) {
			t.Errorf("EOP(%.1f) returned err = %v, want ErrOutOfRange", mjd, err)
		}
	}
}

// A blank length of day is not the same case: it is absent from far more rows
// than the others, it is a rate rather than an offset, and zero is its own
// physical default — no excess over 86400 seconds. Such a row keeps its
// orientation.
func TestBlankLengthOfDayKeepsTheRow(t *testing.T) {
	t.Parallel()

	bulletin := strings.Join([]string{
		finalsLine("60000.00", "0.123456", "0.234567", "-0.0451234", "0.0012"),
		finalsLine("60001.00", "0.123556", "0.234467", "-0.0461234", ""),
		finalsLine("60002.00", "0.123656", "0.234367", "-0.0471234", "0.0014"),
	}, "\n")

	table, err := iers.ParseFinals2000A(strings.NewReader(bulletin))
	if err != nil {
		t.Fatalf("ParseFinals2000A: %v", err)
	}

	if lo, hi := table.Coverage(); lo != 60000 || hi != 60002 {
		t.Errorf("coverage is [%.1f, %.1f], want all three rows", lo, hi)
	}

	got, err := table.EOP(60001)
	if err != nil {
		t.Fatalf("EOP: %v", err)
	}

	if got.LOD != 0 {
		t.Errorf("a blank length of day parsed as %v, want 0", got.LOD)
	}

	if got.DUT1 == 0 {
		t.Error("a row lost its UT1-UTC because its length of day was blank")
	}
}

// A row whose orientation fields hold something that is not a number is the
// same case as a blank one: it is not an orientation.
func TestUnparseableOrientationRowsAreDropped(t *testing.T) {
	t.Parallel()

	bulletin := strings.Join([]string{
		finalsLine("60000.00", "0.123456", "0.234567", "-0.0451234", "0.0012"),
		finalsLine("60001.00", "xxxxxxxx", "0.234467", "-0.0461234", "0.0013"),
		finalsLine("60002.00", "0.123656", "--------", "-0.0471234", "0.0014"),
		finalsLine("60003.00", "0.123756", "0.234267", "n/a", "0.0015"),
		finalsLine("60004.00", "0.123856", "0.234167", "-0.0491234", "0.0016"),
	}, "\n")

	table, err := iers.ParseFinals2000A(strings.NewReader(bulletin))
	if err != nil {
		t.Fatalf("ParseFinals2000A: %v", err)
	}

	// Only the first and last rows are orientations, so nothing in between is
	// covered by a real measurement — but they do bracket the range, and the
	// interpolation across the gap is the honest consequence of the bulletin
	// having a hole rather than a fabrication.
	lo, hi := table.Coverage()
	if lo != 60000 || hi != 60004 {
		t.Errorf("coverage is [%.1f, %.1f], want [60000.0, 60004.0]", lo, hi)
	}

	// The three malformed rows must not be stored as zeros: interpolating the
	// midpoint of the gap must land between the two real values, not at zero.
	mid, err := table.EOP(60002)
	if err != nil {
		t.Fatalf("EOP: %v", err)
	}

	if mid.DUT1 > -0.045 || mid.DUT1 < -0.05 {
		t.Errorf("DUT1 across the gap is %v; it should lie between the two real values "+
			"-0.0451 and -0.0491, not be pulled toward a stored zero", mid.DUT1)
	}
}
