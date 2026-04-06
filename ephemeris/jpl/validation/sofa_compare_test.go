//go:build validation

package jpl_test

import (
	"testing"

	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/time"
)

func runSOFATest(t *testing.T, bid ephemeris.ID) {
	p, err := jpl.NewProvider(jpl.WithSource(jpl.Planets), jpl.WithKernel("de440"), jpl.WithDataDir("../data"))
	if err != nil {
		t.Skipf("skipping SOFA comparison: JPL provider failed: %v", err)
	}
	defer p.Close()

	sofa := ephemeris.Default()

	epochs := []time.Time{
		time.FromJD(2451545.0, time.TDB),
		time.NowUTC(),
		time.Date(2010, 6, 21, 0, 0, 0, 0, time.LocationUTC),
	}

	const sunPosTol = 1e-6
	const moonPosTol = 1e-7

	for i, tm := range epochs {
		t.Run(bid.String(), func(t *testing.T) {
			jplState, err := p.State(bid, tm)
			if err != nil {
				t.Fatalf("JPL State() failed at epoch %d: %v", i, err)
			}

			sofaState, err := sofa.State(bid, tm)
			if err != nil {
				t.Fatalf("SOFA State() failed at epoch %d: %v", i, err)
			}

			posDiff := jplState.Pos.Sub(sofaState.Pos).Norm()
			tol := sunPosTol
			if bid == ephemeris.Moon {
				tol = moonPosTol
			}

			if posDiff > tol {
				t.Errorf("SOFA mismatch at %v: diff=%e AU, want <%e", tm, posDiff, tol)
			}
		})
	}
}

func TestJPLStateAgainstSOFASun(t *testing.T) {
	runSOFATest(t, ephemeris.Sun)
}

func TestJPLStateAgainstSOFAMoon(t *testing.T) {
	runSOFATest(t, ephemeris.Moon)
}
