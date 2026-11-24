package simbad

import (
	"fmt"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// gaiaDR3Row is a star's Gaia DR3 astrometry at J2016.0: its position in
// degrees with errors in mas, its parallax in mas, its proper motion in mas/yr
// and its radial velocity in km/s. DR3 is a fixed release, so a row never
// changes.
type gaiaDR3Row struct {
	ra, dec, raErr, decErr, parallax, pmRA, pmDec, rv float64
}

// gaiaDR3 are the Gaia DR3 rows of the two stars #1248 measured, read from
// ESA's archive on 2026-11-24, keyed by their SIMBAD main_id.
var gaiaDR3 = map[string]gaiaDR3Row{
	"NAME Barnard's star": {
		269.44850252543836, 4.739420051112412, 0.026238997, 0.029043527,
		546.975939730948, -801.5509783684709, 10362.394206546573, -110.46822,
	},
	"*  61 Cyg A": {
		316.7484792940004, 38.76386244649797, 0.03876041, 0.048496146,
		285.99494829578117, 4164.2086922846665, 3249.613883848584, -65.97495,
	},
}

// gaiaRows are the same stars' SIMBAD rows, read from its CfA mirror on
// 2026-11-24. SIMBAD took both positions from Gaia EDR3 and moved them to
// J2000.0.
const gaiaRows = "oid,main_id,ra,dec,otype,pmra,pmdec,plx_value,rvz_radvel," +
	"coo_err_maj,coo_err_min,coo_err_angle,coo_bibcode,pm_err_maj,pm_err_min,pm_err_angle,id\n" +
	`1,NAME Barnard's star,269.4520769586187,4.693364966576667,BY*,-801.551,10362.394,546.9759,-110.11,` +
	`0.0262,0.029,90,2020yCat.1350....0G,0.032,0.036,90,NAME Barnard's star` + "\n" +
	`2,*  61 Cyg A,316.7247482895925,38.74941731943694,BY*,4164.209,3249.614,285.9949,-65.82,` +
	`0.0388,0.0485,90,2020yCat.1350....0G,0.055,0.055,90,*  61 Cyg A` + "\n"

var (
	// gaiaEpoch is Gaia EDR3's and DR3's reference epoch, J2016.0.
	gaiaEpoch = time.FromJD(2457389.0, time.TT)

	// j2026 is J2026.0, a decade after Gaia's epoch.
	j2026 = time.FromJD(2461041.5, time.TT)
)

// moved is where a star at epoch from, with the given motion, is at epoch to.
func moved(t *testing.T, ra, dec, pmRA, pmDec, parallax angle.Angle, rv unit.Velocity, from, to time.Time) coord.ICRS {
	t.Helper()

	c, err := coord.PropagateEpoch(coord.NewICRSWithKinematics(ra, dec, pmRA, pmDec, parallax, rv, from), to)
	testutil.AssertNoError(t, err)

	return c
}

// targetAt is where a target is at epoch, moved from its own epoch with its
// radial velocity.
func targetAt(t *testing.T, tg resolve.Target, epoch time.Time) coord.ICRS {
	t.Helper()

	return moved(t, tg.Coord.RA(), tg.Coord.Dec(), tg.PmRA, tg.PmDec, tg.Parallax, tg.RadialVelocity, tg.Epoch, epoch)
}

// gaiaAt is where a Gaia DR3 row puts its star at epoch.
func gaiaAt(t *testing.T, row gaiaDR3Row, epoch time.Time) coord.ICRS {
	t.Helper()

	mas := func(v float64) angle.Angle { return angle.Arcsec(v / 1000) }

	return moved(t, angle.Deg(row.ra), angle.Deg(row.dec), mas(row.pmRA), mas(row.pmDec), mas(row.parallax),
		unit.KmPerSec(row.rv), gaiaEpoch, epoch)
}

// masApart is the separation of two positions in mas.
func masApart(a, b coord.ICRS) float64 { return coord.Separation(a, b).Arcsec() * 1000 }

// unmovedMas bounds the separation of a position from itself. It is not
// zero on arm64, whose fused multiply-adds round Separation's arithmetic
// differently: 6.3e-9 mas was measured there. Any real move of these stars
// is millions of times larger.
const unmovedMas = 1e-6

// TestAGaiaDerivedPositionIsReportedAtGaiasEpoch is #1248: SIMBAD moves a
// Gaia position to J2000.0 without the radial velocity and keeps the proper
// motion at Gaia's epoch. Read as a star at J2000.0, Barnard's star landed
// 164 mas from its Gaia DR3 row at J2016.0 and 61 Cyg A 34 mas. Reported at
// Gaia's epoch, each lands on its row within Gaia's stated error, and the two
// stay together a decade later, moved with their own radial velocities.
func TestAGaiaDerivedPositionIsReportedAtGaiasEpoch(t *testing.T) {
	t.Parallel()

	targets, err := ParseCSV(strings.NewReader(gaiaRows))
	testutil.AssertNoError(t, err)

	if len(targets) != len(gaiaDR3) {
		t.Fatalf("%d targets, want %d", len(targets), len(gaiaDR3))
	}

	for _, tg := range targets {
		row, ok := gaiaDR3[tg.ID]
		if !ok {
			t.Fatalf("unexpected target %q", tg.ID)
		}

		if !tg.Epoch.Equal(gaiaEpoch) {
			t.Errorf("%s is reported at %v, want Gaia's J2016.0", tg.ID, tg.Epoch)
		}

		atEpoch := masApart(tg.Coord, coord.NewICRS(angle.Deg(row.ra), angle.Deg(row.dec)))
		if bound := row.raErr + row.decErr; atEpoch > bound {
			t.Errorf("%s lands %.4f mas from its Gaia DR3 row at J2016.0, over its stated %.3f mas",
				tg.ID, atEpoch, bound)
		}

		in2026 := masApart(targetAt(t, tg, j2026), gaiaAt(t, row, j2026))
		if in2026 > 1 {
			t.Errorf("%s lands %.4f mas from its Gaia DR3 row at J2026.0, over 1 mas", tg.ID, in2026)
		}

		t.Logf("%s: %.4f mas from Gaia at J2016.0, %.4f mas at J2026.0", tg.ID, atEpoch, in2026)
	}
}

// TestParseBrightReportsAGaiaPositionAtGaiasEpoch: the bright query's rows
// are read the same way.
func TestParseBrightReportsAGaiaPositionAtGaiasEpoch(t *testing.T) {
	t.Parallel()

	targets, err := ParseBrightCSV(strings.NewReader(strings.Replace(gaiaRows, ",id\n", ",vmag\n", 1)))
	testutil.AssertNoError(t, err)

	if len(targets) == 0 {
		t.Fatal("no targets")
	}

	barnard, row := targets[0], gaiaDR3["NAME Barnard's star"]
	if !barnard.Epoch.Equal(gaiaEpoch) {
		t.Errorf("Barnard's star is reported at %v, want Gaia's J2016.0", barnard.Epoch)
	}

	if d := masApart(barnard.Coord, coord.NewICRS(angle.Deg(row.ra), angle.Deg(row.dec))); d > row.raErr+row.decErr {
		t.Errorf("Barnard's star lands %.4f mas from its Gaia DR3 row", d)
	}
}

// TestAPositionFromElsewhereStaysAtJ2000: a position SIMBAD did not take
// from Gaia holds at J2000.0, as SIMBAD gives it.
func TestAPositionFromElsewhereStaysAtJ2000(t *testing.T) {
	t.Parallel()

	targets, err := ParseCSV(strings.NewReader(errorRows))
	testutil.AssertNoError(t, err)

	for _, tg := range targets {
		if tg.ID != "* eps Ori" {
			continue
		}

		if !tg.Epoch.Equal(time.J2000()) {
			t.Errorf("a Hipparcos position is reported at %v, want J2000.0", tg.Epoch)
		}

		if d := masApart(tg.Coord, coord.NewICRS(angle.Deg(84.05338894077023), angle.Deg(-1.2019191358333312))); d > unmovedMas {
			t.Errorf("a Hipparcos position moved %g mas", d)
		}

		return
	}

	t.Fatal("eps Ori is missing")
}

// gaiaRow is one SIMBAD row whose position comes from the Gaia release
// bibcode cites, with an empty proper motion when pmRA and pmDec are both
// empty strings.
func gaiaRow(ra, dec float64, pmRA, pmDec, parallax, bibcode string) string {
	return "oid,main_id,ra,dec,otype,pmra,pmdec,plx_value,rvz_radvel,coo_bibcode,id\n" +
		fmt.Sprintf("1,star,%.15g,%.15g,*,%s,%s,%s,40,%s,star\n", ra, dec, pmRA, pmDec, parallax, bibcode)
}

// TestTheGaiaEpochPositionMovesBackOntoSIMBADs: moving the position found at
// Gaia's epoch as SIMBAD does, without the radial velocity, gives SIMBAD's
// position back within a microarcsecond: 11 arcseconds from a pole, across
// RA 0h, for a star that does not move, and at Gaia DR2's epoch.
func TestTheGaiaEpochPositionMovesBackOntoSIMBADs(t *testing.T) {
	t.Parallel()

	const edr3, dr2 = "2020yCat.1350....0G", "2018yCat.1345....0G"

	dr2Epoch := time.FromJD(2457206.375, time.TT)

	for _, tc := range []struct {
		name     string
		ra, dec  float64
		pm, pmD  string
		parallax string
		bibcode  string
		epoch    time.Time
	}{
		{"Barnard's star", 269.4520769586187, 4.693364966576667, "-801.551", "10362.394", "546.9759", edr3, gaiaEpoch},
		{"11 arcseconds from the north pole", 123.4, 89.997, "3", "-4", "100", edr3, gaiaEpoch},
		{"across RA 0h", 359.99995, -20, "9000", "0", "300", edr3, gaiaEpoch},
		{"a star with no proper motion", 10, 10, "", "", "", edr3, gaiaEpoch},
		{"Gaia DR2's epoch", 269.4520769586187, 4.693364966576667, "-801.551", "10362.394", "546.9759", dr2, dr2Epoch},
	} {
		targets, err := ParseCSV(strings.NewReader(gaiaRow(tc.ra, tc.dec, tc.pm, tc.pmD, tc.parallax, tc.bibcode)))
		testutil.AssertNoError(t, err)

		if len(targets) != 1 {
			t.Fatalf("%s: %d targets, want 1", tc.name, len(targets))
		}

		tg := targets[0]
		if !tg.Epoch.Equal(tc.epoch) {
			t.Errorf("%s: reported at %v, want %v", tc.name, tg.Epoch, tc.epoch)

			continue
		}

		simbad := coord.NewICRS(angle.Deg(tc.ra), angle.Deg(tc.dec))
		back := moved(t, tg.Coord.RA(), tg.Coord.Dec(), tg.PmRA, tg.PmDec, tg.Parallax, unit.KmPerSec(0), tg.Epoch, time.J2000())

		if d := masApart(back, simbad); d > 1e-3 {
			t.Errorf("%s: moved back to J2000.0 without the radial velocity, it lands %.6f mas from SIMBAD's position",
				tc.name, d)
		}
	}
}

// TestAMotionTheSearchMayNotUndoLeavesSIMBADsPositionOrUndoesIt: for a star
// whose motion between the epochs carries it across a pole, and for a proper
// motion of a thousand degrees a year, which is no star's, the search need
// not converge. Either way the target is at Gaia's epoch where SIMBAD's move
// takes it back onto SIMBAD's position, or at J2000.0 where SIMBAD put it.
// Which of the two is not pinned, since it can turn on the last bit.
func TestAMotionTheSearchMayNotUndoLeavesSIMBADsPositionOrUndoesIt(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name             string
		ra, dec          float64
		pmRA, pmDec, plx string
	}{
		{"80 arcseconds of motion across the north pole", 123.4, 89.99999, "3000", "-4000", "100"},
		{"a thousand degrees a year", 45, 30, "3.6e9", "-3.6e9", "1"},
	} {
		targets, err := ParseCSV(strings.NewReader(gaiaRow(tc.ra, tc.dec, tc.pmRA, tc.pmDec, tc.plx, "2020yCat.1350....0G")))
		testutil.AssertNoError(t, err)

		if len(targets) != 1 {
			t.Fatalf("%s: %d targets, want 1", tc.name, len(targets))
		}

		tg, simbad := targets[0], coord.NewICRS(angle.Deg(tc.ra), angle.Deg(tc.dec))

		switch {
		case tg.Epoch.Equal(time.J2000()):
			if d := masApart(tg.Coord, simbad); d > unmovedMas {
				t.Errorf("%s: left at J2000.0, it moved %g mas", tc.name, d)
			}
		case tg.Epoch.Equal(gaiaEpoch):
			back := moved(t, tg.Coord.RA(), tg.Coord.Dec(), tg.PmRA, tg.PmDec, tg.Parallax, unit.KmPerSec(0), tg.Epoch, time.J2000())
			if d := masApart(back, simbad); d > 1e-3 {
				t.Errorf("%s: reported at Gaia's epoch, it moves back %.6f mas from SIMBAD's position", tc.name, d)
			}
		default:
			t.Errorf("%s: reported at %v, neither J2000.0 nor Gaia's epoch", tc.name, tg.Epoch)
		}
	}
}
