package mpcorb_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/mpcorb"
	"github.com/TuSKan/astrogo/catalog/resolve"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// cometRows are verbatim rows of the MPC's CometEls.txt, fetched 2026-09-23.
// Each is here for something it breaks:
//
//   - C/1995 O1 (Hale-Bopp) — the ordinary long-period comet, and a name in
//     parentheses after the designation.
//   - C/2023 A3 — e = 1.000178, the open orbit this reader exists for.
//   - C/2014 R3 — e = 1.000000 exactly, the one parabola in the file.
//   - 1P/Halley — a numbered periodic comet: the number fills columns 1-4
//     and the provisional designation is blank.
//   - 3D-A/Biela — a fragment, whose letter sits alone in column 12.
//   - 332P-B — an unperturbed solution, with no epoch at all.
//   - A/2019 N2 — an asteroid on a cometary orbit.
//   - C/2014 UN271 — q = 10.96 AU, which fills all nine columns of its field;
//     every other row leaves the first one blank.
//   - 1I/'Oumuamua — interstellar, and a backtick in its name.
var cometRows = []string{
	"    CJ95O010  1997 03 29.0319  0.925246  0.994897  130.7250  281.8047   89.7360  20260922  -2.0  4.0  C/1995 O1 (Hale-Bopp)                                    MPC194091",
	"    CK23A030  2024 09 27.8264  0.391333  1.000178  308.5800   21.6698  139.1001  20260922   6.5  3.2  C/2023 A3 (Tsuchinshan-ATLAS)                            MPEC 2026-O98",
	"    CK14R030  2016 08  8.3994  7.266681  1.000000  113.3732  333.9781   90.7547  20260922   6.5  4.0  C/2014 R3 (PANSTARRS)                                    MPC194139",
	"0001P         2061 08  2.3518  0.571051  0.968024  112.1785   59.2781  162.1895  20260922   5.5  3.2  1P/Halley                                                MPC191592",
	"0003D      a  2024 12  2.1391  0.827575  0.767046  192.1556  276.6851    7.2142  20260922  11.0  6.0  3D-A/Biela                                                76, 1135",
	"0332P      b  2016 03 17.1650  1.572891  0.489707  152.3711    3.8028    9.3805            19.0  4.0  332P-B/Ikeya-Murakami                                    MPEC 2016-EI8",
	"    AK19N020  2019 08 21.3110  1.920373  0.974329  346.4995  276.3980   89.5520  20260922  12.5  2.0  A/2019 N2                                                MPC194162",
	"    CK14UR1N  2031 01 16.6358 10.959407  1.004072  326.0935  190.0054   95.4459  20260922   2.5  3.2  C/2014 UN271 (Bernardinelli-Bernstein)                   MPEC 2026-RA1",
	"0001I         2017 09  9.4886  0.255240  1.199252  241.6845   24.5997  122.6778  20170904  23.0  2.0  1I/`Oumuamua                                             MPC107687",
}

func cometFixture() string { return strings.Join(cometRows, "\n") + "\n" }

// TestReadParsesCometRows checks every field of the comet format against rows
// read by hand. The expected Julian Dates were computed separately from the
// calendar dates in the rows, not through astrogo.
func TestReadParsesCometRows(t *testing.T) {
	list := readAll(t, cometFixture())
	if len(list) != len(cometRows) {
		t.Fatalf("read %d targets from %d comet rows", len(list), len(cometRows))
	}

	cases := []struct {
		id, name, designation string
		kind                  resolve.Kind
		q, e                  float64
		incl, node, argPeri   float64
		tpJD, epochJD         float64
		m1, k1                float64
	}{
		{"CJ95O010", "C/1995 O1 (Hale-Bopp)", "C/1995 O1", resolve.KindComet,
			0.925246, 0.994897, 89.7360, 281.8047, 130.7250, 2450536.5319, 2461305.5, -2.0, 10.0},
		{"CK23A030", "C/2023 A3 (Tsuchinshan-ATLAS)", "C/2023 A3", resolve.KindComet,
			0.391333, 1.000178, 139.1001, 21.6698, 308.5800, 2460581.3264, 2461305.5, 6.5, 8.0},
		{"CK14R030", "C/2014 R3 (PANSTARRS)", "C/2014 R3", resolve.KindComet,
			7.266681, 1.0, 90.7547, 333.9781, 113.3732, 2457608.8994, 2461305.5, 6.5, 10.0},
		{"0001P", "1P/Halley", "1P/Halley", resolve.KindComet,
			0.571051, 0.968024, 162.1895, 59.2781, 112.1785, 2474038.8518, 2461305.5, 5.5, 8.0},
		{"0332Pb", "332P-B/Ikeya-Murakami", "332P-B/Ikeya-Murakami", resolve.KindComet,
			1.572891, 0.489707, 9.3805, 3.8028, 152.3711, 2457464.665, 2457464.665, 19.0, 10.0},
		{"AK19N020", "A/2019 N2", "A/2019 N2", resolve.KindAsteroid,
			1.920373, 0.974329, 89.5520, 276.3980, 346.4995, 2458716.8110, 2461305.5, 12.5, 5.0},
		{"CK14UR1N", "C/2014 UN271 (Bernardinelli-Bernstein)", "C/2014 UN271", resolve.KindComet,
			10.959407, 1.004072, 95.4459, 190.0054, 326.0935, 2462883.1358, 2461305.5, 2.5, 8.0},
		{"0001I", "1I/`Oumuamua", "1I/`Oumuamua", resolve.KindInterstellar,
			0.255240, 1.199252, 122.6778, 24.5997, 241.6845, 2458005.9886, 2458000.5, 23.0, 5.0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := find(t, list, tc.id)

			testutil.AssertEqual(t, "Name", c.Name, tc.name)
			testutil.AssertEqual(t, "Designation", c.Designation, tc.designation)
			testutil.AssertEqual(t, "Kind", c.Kind, tc.kind)
			testutil.AssertEqual(t, "Catalog", c.Catalog, "mpcorb")

			if !c.HasElements || !c.HasM1 {
				t.Fatalf("HasElements = %v, HasM1 = %v; a comet row carries both", c.HasElements, c.HasM1)
			}

			testutil.AssertNear(t, "q", c.PerihelionDistance.AU(), tc.q, 1e-12)
			testutil.AssertNear(t, "e", c.Eccentricity, tc.e, 1e-12)
			testutil.AssertNear(t, "i", c.Inclination.Degrees(), tc.incl, 1e-12)
			testutil.AssertNear(t, "node", c.AscendingNode.Degrees(), tc.node, 1e-12)
			testutil.AssertNear(t, "argument of perihelion", c.ArgPeriapsis.Degrees(), tc.argPeri, 1e-12)

			// 1e-8 day is under a millisecond; the row gives the day to 8.6 s.
			testutil.AssertNear(t, "perihelion JD", c.PerihelionTime.JD(), tc.tpJD, 1e-8)
			testutil.AssertNear(t, "epoch JD", c.Epoch.JD(), tc.epochJD, 1e-8)

			testutil.AssertEqual(t, "perihelion scale", c.PerihelionTime.Scale().String(), "TT")

			testutil.AssertNear(t, "M1", c.M1, tc.m1, 1e-12)
			testutil.AssertNear(t, "K1 = 2.5 × slope", c.K1, tc.k1, 1e-12)

			// The file does not publish the asteroid form, and an open orbit
			// has none; it is left at zero rather than derived.
			if c.SemiMajorAxis != 0 || c.MeanAnomaly.Radians() != 0 {
				t.Errorf("SemiMajorAxis = %v, MeanAnomaly = %v; a comet row sets neither",
					c.SemiMajorAxis, c.MeanAnomaly)
			}
		})
	}
}

// TestReadKeepsTheFragmentLetter checks the one row whose identity is split
// across columns: Biela's fragment A is "0003D" plus an "a" in column 12.
func TestReadKeepsTheFragmentLetter(t *testing.T) {
	biela := find(t, readAll(t, cometFixture()), "0003Da")
	testutil.AssertEqual(t, "Name", biela.Name, "3D-A/Biela")
}

// TestReadReadsAFileOfBothFormats is the claim the package doc makes: rows are
// recognized by their shape, so MPCORB rows and comet rows read through one
// call, around header lines that are neither.
func TestReadReadsAFileOfBothFormats(t *testing.T) {
	header := "Header line of no particular shape\n" + strings.Repeat("-", 160) + "\n\n"

	list := readAll(t, header+fixture+cometFixture())

	var comets, asteroids int

	for _, tgt := range list {
		switch {
		case tgt.PerihelionDistance > 0 && tgt.SemiMajorAxis == 0:
			comets++
		case tgt.SemiMajorAxis > 0 && tgt.PerihelionDistance == 0:
			asteroids++
		default:
			t.Errorf("%s carries both element forms or neither", tgt.ID)
		}
	}

	testutil.AssertEqual(t, "comet rows", comets, len(cometRows))
	testutil.AssertEqual(t, "MPCORB rows", asteroids, strings.Count(fixture, "\n"))
}

// TestEveryCometRowPropagates is the end-to-end claim for comets: what the
// reader produces goes straight into the perihelion-form constructor, for
// every conic in the fixture, and puts the body at its own perihelion
// distance at its own perihelion time.
func TestEveryCometRowPropagates(t *testing.T) {
	for _, c := range readAll(t, cometFixture()) {
		el, err := eph.ElementsFromPerihelion(c.PerihelionTime, c.PerihelionDistance, c.Eccentricity,
			c.Inclination, c.AscendingNode, c.ArgPeriapsis)
		if err != nil {
			t.Fatalf("%s: ElementsFromPerihelion: %v", c.Name, err)
		}

		pos, _, err := el.StateAt(c.PerihelionTime)
		if err != nil {
			t.Fatalf("%s: StateAt its perihelion: %v", c.Name, err)
		}

		if d := math.Abs(pos.Norm() - c.PerihelionDistance.AU()); d > 1e-12 {
			t.Errorf("%s: %.12f AU from the Sun at perihelion, want q = %.12f", c.Name, pos.Norm(), c.PerihelionDistance.AU())
		}
	}
}

// TestReadYieldsABadCometRowAndKeepsGoing checks a comet row that is the right
// shape with a field that is not a number, and one whose date cannot be a
// date: each is an ErrMalformedRow for that row alone.
func TestReadYieldsABadCometRowAndKeepsGoing(t *testing.T) {
	good := cometRows[0]

	badQ := good[:31] + "0.9x5246" + good[39:]
	badMonth := good[:19] + "13" + good[21:]
	badEpoch := good[:81] + "20261399" + good[89:]

	for name, row := range map[string]string{"q": badQ, "month": badMonth, "epoch": badEpoch} {
		var (
			errs []error
			read int
		)

		for tgt, err := range mpcorb.Read(strings.NewReader(row + "\n" + cometRows[1] + "\n")) {
			if errors.Is(err, mpcorb.ErrMalformedRow) {
				errs = append(errs, err)

				continue
			}

			if err != nil {
				t.Fatalf("Read: %v", err)
			}

			read++

			testutil.AssertEqual(t, "the row after the bad one", tgt.ID, "CK23A030")
		}

		if len(errs) != 1 || !errors.Is(errs[0], mpcorb.ErrMalformedRow) {
			t.Errorf("bad %s: errors %v, want one ErrMalformedRow", name, errs)
		}

		testutil.AssertEqual(t, "bad "+name+": rows read after it", read, 1)
	}
}
