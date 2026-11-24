package simbad

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// errorRows are three of SIMBAD's basic rows, read 2026-11-18: Barnard's
// star, whose position SIMBAD took from Gaia EDR3; eps Ori, from Hipparcos;
// and M 31, which has no error.
const errorRows = "oid,main_id,ra,dec,otype,pmra,pmdec,plx_value,rvz_radvel," +
	"coo_err_maj,coo_err_min,coo_err_angle,coo_bibcode,pm_err_maj,pm_err_min,pm_err_angle,id\n" +
	`1,NAME Barnard's star,269.4520769586187,4.693364966576667,BY*,-801.551,10362.394,546.9759,-110.11,` +
	`0.0262,0.029,90,2020yCat.1350....0G,0.032,0.036,90,NAME Barnard's star` + "\n" +
	`2,* eps Ori,84.05338894077023,-1.2019191358333312,s*b,1.44,-0.78,1.65,27.3,` +
	`3.6897864,1.6733218,90,2007A&A...474..653V,0.42,0.19,0,* eps Ori` + "\n" +
	`3,M  31,10.684708333333333,41.268750000000004,AGN,,,,-300,,,,2006AJ....131.1163S,,,,M  31` + "\n"

// masOf checks that got, an angle in arcseconds, is want milliarcseconds.
func masOf(t *testing.T, label string, got, want float64) {
	t.Helper()
	testutil.AssertRelNear(t, label, got*1000, want, 1e-9)
}

// TestParseReadsTheErrorEllipses: an ellipse whose major axis lies east
// (90°) has its major axis as the α* error and its minor as the δ error,
// one whose major axis lies north (0°) the other way round; a row with no
// ellipse has no error; and a position SIMBAD took from Gaia carries its
// errors at Gaia's epoch.
func TestParseReadsTheErrorEllipses(t *testing.T) {
	t.Parallel()

	targets, err := ParseCSV(strings.NewReader(errorRows))
	testutil.AssertNoError(t, err)

	byID := make(map[string]resolve.Target, len(targets))
	for _, tg := range targets {
		byID[tg.ID] = tg
	}

	barnard := byID["NAME Barnard's star"].AstrometricError
	masOf(t, "Barnard σ(α*)", barnard.RA.Arcseconds(), 0.0262)
	masOf(t, "Barnard σ(δ)", barnard.Dec.Arcseconds(), 0.029)
	masOf(t, "Barnard σ(μα*)", barnard.PmRA.Arcseconds(), 0.032)
	masOf(t, "Barnard σ(μδ)", barnard.PmDec.Arcseconds(), 0.036)

	if math.Abs(barnard.RADec) > 1e-12 || math.Abs(barnard.PmRAPmDec) > 1e-12 {
		t.Errorf("an axis-aligned ellipse is uncorrelated: got %g, %g", barnard.RADec, barnard.PmRAPmDec)
	}

	if want := epoch.FromJD(2457389.0, epoch.TT); !barnard.Epoch.Equal(want) {
		t.Errorf("Barnard's errors hold at %v, want Gaia EDR3's J2016.0", barnard.Epoch)
	}

	epsOri := byID["* eps Ori"].AstrometricError
	masOf(t, "eps Ori σ(α*)", epsOri.RA.Arcseconds(), 3.6897864)
	masOf(t, "eps Ori σ(δ)", epsOri.Dec.Arcseconds(), 1.6733218)
	masOf(t, "eps Ori σ(μα*), a minor axis east", epsOri.PmRA.Arcseconds(), 0.19)
	masOf(t, "eps Ori σ(μδ), a major axis north", epsOri.PmDec.Arcseconds(), 0.42)

	if !epsOri.Epoch.IsZero() {
		t.Errorf("a Hipparcos position's errors hold at the row's own epoch, got %v", epsOri.Epoch)
	}

	if m31 := byID["M  31"].AstrometricError; m31 != (resolve.AstrometricError{}) {
		t.Errorf("M 31 publishes no error, got %+v", m31)
	}
}

// TestParseBrightReadsTheErrorEllipses: the bright query's rows carry the
// same errors.
func TestParseBrightReadsTheErrorEllipses(t *testing.T) {
	t.Parallel()

	targets, err := ParseBrightCSV(strings.NewReader(strings.Replace(errorRows, ",id\n", ",vmag\n", 1)))
	testutil.AssertNoError(t, err)

	if len(targets) != 3 {
		t.Fatalf("%d targets, want 3", len(targets))
	}

	masOf(t, "Barnard σ(δ)", targets[0].AstrometricError.Dec.Arcseconds(), 0.029)
}

// TestErrorEllipseRotates: a 3 × 1 mas ellipse with its major axis 30° east
// of north has var(α*) = 9 sin²30° + cos²30° = 3, var(δ) = 9 cos²30° +
// sin²30° = 7, and covariance 8 sin 30° cos 30°, a correlation of
// 2√3/√21.
func TestErrorEllipseRotates(t *testing.T) {
	t.Parallel()

	ra, dec, corr := errorEllipse(3, 1, 30)
	masOf(t, "σ(α*)", ra.Arcseconds(), math.Sqrt(3))
	masOf(t, "σ(δ)", dec.Arcseconds(), math.Sqrt(7))
	testutil.AssertRelNear(t, "correlation", corr, 2*math.Sqrt(3)/math.Sqrt(21), 1e-12)
}
