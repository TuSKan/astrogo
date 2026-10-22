//go:build integration

package jpl_test

import (
	"context"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/time"
)

// TestLocateAnswersAboutTheSegmentCenter: a kernel locates a body about the
// center of the segment that holds it, as de440s stores it — the Moon and the
// Earth about the Earth-Moon barycenter, the barycenters and the Sun about the
// Solar System barycenter — and an Instant composed from those reproduces the
// barycentric route to rounding: the Moon about the Earth through their
// barycenter, and through the Solar System barycenter an AU away.
func TestLocateAnswersAboutTheSegmentCenter(t *testing.T) {
	t.Parallel()

	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440s")
	if err != nil {
		t.Fatalf("de440s: %v", err)
	}

	t.Cleanup(func() { _ = p.Close() })

	at := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC)

	for _, c := range []struct {
		id, center core.ID
	}{
		{core.Moon, core.EarthMoonBarycenter},
		{core.Earth, core.EarthMoonBarycenter},
		{core.EarthMoonBarycenter, core.SolarSystemBarycenter},
		{core.Sun, core.SolarSystemBarycenter},
		{core.MarsBarycenter, core.SolarSystemBarycenter},
	} {
		st, err := p.Locate(c.id, at)
		if err != nil {
			t.Fatalf("Locate(%v): %v", c.id, err)
		}

		if st.Center != c.center || st.Frame != core.FrameICRS {
			t.Errorf("Locate(%v) is about %v on %s axes, want about %v on the ICRS's", c.id, st.Center, st.Frame, c.center)
		}
	}

	in := eph.At(p, at)

	native, err := in.Geometric(core.Moon, core.Earth)
	if err != nil {
		t.Fatal(err)
	}

	moon, err := in.Geometric(core.Moon, core.SolarSystemBarycenter)
	if err != nil {
		t.Fatal(err)
	}

	earth, err := in.Geometric(core.Earth, core.SolarSystemBarycenter)
	if err != nil {
		t.Fatal(err)
	}

	// The barycentric route rounds at an AU, 2e-16 AU, three centimeters;
	// the native one at the Moon's distance.
	if d := moon.Pos.Sub(earth.Pos).Sub(native.Pos).Norm(); d > 1e-15 {
		t.Errorf("the Moon about the Earth natively and through the barycenter differ by %g AU", d)
	}
}
