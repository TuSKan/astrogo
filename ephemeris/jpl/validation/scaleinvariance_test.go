//go:build validation

package jpl_test

import (
	"context"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/internal/metrology"
	"github.com/TuSKan/astrogo/time"
)

// metresInAU converts the tolerance below into the AU that State reports in.
const metresInAU = 1.0 / 149597870700.0

// jplScaleTolerance is one metre, for the reasons given on the offline half of
// this test in ephemeris/scaleinvariance_test.go: the expected difference is
// zero, and the tolerance absorbs only the float cost of a scale round-trip.
const jplScaleTolerance = 1.0 * metresInAU

// TestJPLStateIsScaleInvariant is the kernel-backed half of the contract that
// ephemeris.TestProviderStateIsScaleInvariant asserts for SOFA and SGP4.
//
// It lives here rather than beside them because it needs a real DE440 kernel,
// and it is the case that actually failed: lsk.UTCToTDB special-cased only
// Scale() == TDB and treated everything else as UTC, so a TT input had leap
// seconds plus 32.184 s added on top of an offset it already carried — 69.184 s
// late, about 40 arcsec of lunar motion and 1.8 arcsec for Mars.
//
// The Moon is the sensitive body here: it moves ~0.55 arcsec/s against the
// stars, so an epoch slip shows up an order of magnitude larger than it does
// for a planet. A test that only checked Mars would have found this defect ~20
// times smaller and might have been written with a tolerance that hid it.
func TestJPLStateIsScaleInvariant(t *testing.T) {
	suite := metrology.NewSuite("ephemeris.jpl.scaleinvariance",
		sofaReference(), contractFor(eph.Moon))

	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440")
	if err != nil {
		metrology.NotVerified(t, "the JPL provider could not be built: "+err.Error(), suite)
	}

	defer func() { _ = p.Close() }()

	// Fixed, and deliberately clear of a leap-second boundary: the point here
	// is the provider's handling of the caller's label, not the leap-second
	// table's behaviour at a discontinuity.
	utc := time.Date(2026, time.April, 20, 3, 0, 0, 0, time.LocationUTC)

	bodies := map[string]eph.ID{
		"Moon": eph.Moon,
		"Mars": eph.Mars,
		"Sun":  eph.Sun,
	}

	scales := []struct {
		label string
		at    func(time.Time) time.Time
	}{
		{"UTC", func(t time.Time) time.Time { return t.UTC() }},
		{"TAI", func(t time.Time) time.Time { return t.TAI() }},
		{"TT", func(t time.Time) time.Time { return t.TT() }},
		{"TDB", func(t time.Time) time.Time { return t.TDB() }},
	}

	for name, id := range bodies {
		t.Run(name, func(t *testing.T) {
			base, err := p.State(id, utc)
			if err != nil {
				t.Fatalf("State at the reference epoch: %v", err)
			}

			for _, s := range scales {
				got, err := p.State(id, s.at(utc))
				if err != nil {
					t.Fatalf("State with a %s-labelled instant: %v", s.label, err)
				}

				if d := got.Pos.Sub(base.Pos).Norm(); d > jplScaleTolerance {
					t.Errorf("position moved %.6g AU (%.4g km) when the same instant "+
						"was labelled %s.\n  The provider is reading the caller's scale "+
						"as its own — normalise at the entry point rather than reading "+
						"JDParts raw.", d, d/metresInAU/1e3, s.label)
				}
			}
		})
	}
}
