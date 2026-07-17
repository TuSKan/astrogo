package iers

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestParseFinals2000A(t *testing.T) {
	// Sample data mimicking finals2000A.all format for two consecutive days.
	data := []byte(`73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300   .143000   .137000   .8075000   -18.637    -3.667  
73 1 3 41685.00 I  0.118980 0.011039  0.135656 0.013616  I 0.8056163 0.0002710  3.5563 0.1916  P    -0.751    0.199    -0.701    0.300   .141000   .134000   .8044000   -18.636    -3.571  `)

	table, err := ParseFinals2000A(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error parsing dataset: %v", err)
	}

	if len(table.records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(table.records))
	}

	r0 := table.records[0]
	testutil.AssertEqual(t, "MJD Record 0", r0.MJD, 41684.0)

	arcsec2rad := math.Pi / (180.0 * 3600.0)
	testutil.AssertNear(t, "XP Record 0", r0.XP, 0.120733*arcsec2rad, 1e-12)
	testutil.AssertNear(t, "YP Record 0", r0.YP, 0.136966*arcsec2rad, 1e-12)
	testutil.AssertEqual(t, "DUT1 Record 0", r0.DUT1, 0.8084178)
	testutil.AssertEqual(t, "LOD Record 0", r0.LOD, 0.0)

	r1 := table.records[1]
	testutil.AssertEqual(t, "LOD Record 1", r1.LOD, 3.5563)

	// Test exact bounds interpolation
	eopZero, err := table.EOP(41684.0)
	if err != nil {
		t.Fatal(err)
	}

	testutil.AssertEqual(t, "EOP Exact Bound DUT1", eopZero.DUT1, 0.8084178)

	// Test linear interpolation (midpoint)
	eopMid, err := table.EOP(41684.5)
	if err != nil {
		t.Fatal(err)
	}

	expectedDUT1 := (0.8084178 + 0.8056163) / 2.0
	testutil.AssertNear(t, "EOP Midpoint DUT1", eopMid.DUT1, expectedDUT1, 1e-12)

	expectedXP := (0.120733 + 0.118980) / 2.0 * arcsec2rad
	testutil.AssertNear(t, "EOP Midpoint XP", eopMid.XP, expectedXP, 1e-12)

	// Test out-of-range queries return ErrOutOfRange
	_, err = table.EOP(40000.0)
	if err == nil {
		t.Error("expected ErrOutOfRange for MJD below coverage, got nil")
	} else if !errors.Is(err, ErrOutOfRange) {
		t.Errorf("expected ErrOutOfRange, got: %v", err)
	}

	_, err = table.EOP(50000.0)
	if err == nil {
		t.Error("expected ErrOutOfRange for MJD above coverage, got nil")
	} else if !errors.Is(err, ErrOutOfRange) {
		t.Errorf("expected ErrOutOfRange, got: %v", err)
	}

	// Test Coverage()
	minMJD, maxMJD := table.Coverage()
	testutil.AssertEqual(t, "Coverage min", minMJD, 41684.0)
	testutil.AssertEqual(t, "Coverage max", maxMJD, 41685.0)
}

// TestPackageCoverage exercises the package-level Coverage function against
// both a real *Table (ok=true, matching the table's own Coverage()) and
// ZeroModel (ok=false, since it has no epoch-dependent range to report).
func TestPackageCoverage(t *testing.T) {
	orig := GetModel()
	defer RegisterModel(orig)

	data := []byte(`73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300   .143000   .137000   .8075000   -18.637    -3.667
73 1 3 41685.00 I  0.118980 0.011039  0.135656 0.013616  I 0.8056163 0.0002710  3.5563 0.1916  P    -0.751    0.199    -0.701    0.300   .141000   .134000   .8044000   -18.636    -3.571  `)

	table, err := ParseFinals2000A(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("unexpected error parsing dataset: %v", err)
	}

	RegisterModel(table)

	minMJD, maxMJD, ok := Coverage()
	if !ok {
		t.Fatal("Coverage(): expected ok=true for a registered *Table")
	}

	testutil.AssertEqual(t, "package Coverage min", minMJD, 41684.0)
	testutil.AssertEqual(t, "package Coverage max", maxMJD, 41685.0)

	RegisterModel(ZeroModel{})

	if _, _, ok := Coverage(); ok {
		t.Error("Coverage(): expected ok=false for ZeroModel")
	}
}
