package gaia

import (
	"slices"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// barnardRow is Barnard's star's Gaia DR3 row as the archive answered the
// cone query's error columns, read 2026-11-18.
const barnardRow = "source_id,ra,dec,pmra,pmdec,parallax,ra_error,dec_error,pmra_error,pmdec_error," +
	"ra_dec_corr,ra_pmra_corr,ra_pmdec_corr,dec_pmra_corr,dec_pmdec_corr,pmra_pmdec_corr\n" +
	"4472832130942575872,269.44850252543836,4.739420051112412,-801.5509783684709,10362.394206546573,546.9759," +
	"0.026238997,0.029043527,0.031820867,0.036070455," +
	"-0.052255288,-0.21109906,-0.03466423,-0.05581326,-0.63121563,0.03058501\n"

// TestParseReadsTheAstrometricErrors: a row's errors, in mas and mas/yr,
// and its six correlations are the Target's, holding at the row's own
// epoch.
func TestParseReadsTheAstrometricErrors(t *testing.T) {
	t.Parallel()

	targets, _, err := parseResult(strings.NewReader(barnardRow))
	testutil.AssertNoError(t, err)

	if len(targets) != 1 {
		t.Fatalf("%d targets, want 1", len(targets))
	}

	e := targets[0].AstrometricError

	for _, tc := range []struct {
		name      string
		got, want float64
	}{
		{"σ(α*)", e.RA.Arcseconds() * 1000, 0.026238997},
		{"σ(δ)", e.Dec.Arcseconds() * 1000, 0.029043527},
		{"σ(μα*)", e.PmRA.Arcseconds() * 1000, 0.031820867},
		{"σ(μδ)", e.PmDec.Arcseconds() * 1000, 0.036070455},
		{"ρ(α*, δ)", e.RADec, -0.052255288},
		{"ρ(α*, μα*)", e.RAPmRA, -0.21109906},
		{"ρ(α*, μδ)", e.RAPmDec, -0.03466423},
		{"ρ(δ, μα*)", e.DecPmRA, -0.05581326},
		{"ρ(δ, μδ)", e.DecPmDec, -0.63121563},
		{"ρ(μα*, μδ)", e.PmRAPmDec, 0.03058501},
	} {
		testutil.AssertRelNear(t, tc.name, tc.got, tc.want, 1e-12)
	}

	if !e.Epoch.IsZero() {
		t.Errorf("Gaia's errors hold at the row's epoch; got their own %v", e.Epoch)
	}
}

// TestARowIsKnownByItsDesignation: a row carries Gaia's designation, so it
// shares an identity key with another catalog's cross-identification of
// the source, as SIMBAD lists it.
func TestARowIsKnownByItsDesignation(t *testing.T) {
	t.Parallel()

	targets, _, err := parseResult(strings.NewReader(barnardRow))
	testutil.AssertNoError(t, err)

	simbad := resolve.IdentityKeys(resolve.Target{ID: "NAME Barnard's star", Aliases: []string{"Gaia DR3 4472832130942575872"}})
	if !slices.ContainsFunc(resolve.IdentityKeys(targets[0]), func(k string) bool { return slices.Contains(simbad, k) }) {
		t.Errorf("Gaia keys %v share none with SIMBAD's %v", resolve.IdentityKeys(targets[0]), simbad)
	}
}
