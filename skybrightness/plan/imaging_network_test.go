//go:build network

package plan_test

import (
	"context"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/optics"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset"
	sbplan "github.com/TuSKan/astrogo/skybrightness/plan"
	"github.com/TuSKan/astrogo/time"
)

// The whole chain, against a real modelled sky.
//
// # What only this test can say
//
// That the pieces compose. The arithmetic is checked offline, the constraint
// is checked against a stub, and neither notices if the units handed across
// the boundary disagree — a surface brightness per square arcsecond meeting a
// pixel solid angle in steradians produces a number either way, and only a
// real evaluation puts a plausible magnitude at one end of it.
//
// It also asserts the physical claim the whole exercise is for: a brighter
// sky is a shallower limit. At ten degrees of altitude Paranal's sky is about
// seven tenths of a magnitude brighter than at the zenith, and an instrument
// that could not see the difference would make this entire wiring pointless.
func TestImagingDepthAgainstARealSky(t *testing.T) {
	ids, size := dataset.Endpoints(skybrightness.GAMBONSWeb)
	remote.EnableDownloads(size, ids...)

	site, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	sky, err := dataset.Open(context.Background(), dataset.Spec{Preset: skybrightness.GAMBONSWeb})
	if err != nil {
		t.Skipf("no sky available: %v", err)
	}

	// A 200 mm f/5 with 1.5-arcsecond pixels and ordinary CMOS noise. The
	// absolute depth depends on all of it, which is why none of it is
	// defaulted anywhere in this package.
	//
	// No throughput elements, so this is a perfectly transmitting, filterless
	// instrument: the depth below is an optimistic bound rather than a
	// prediction, and the zenith-to-ten-degrees difference is smaller than
	// the V-band sky difference for the reason LimitingMagnitudeAt sets out
	// — broadband electrons against a V-band magnitude scale.
	scope, err := optics.NewTelescope(200, 1000)
	if err != nil {
		t.Fatalf("NewTelescope: %v", err)
	}

	inst, err := optics.NewInstrument("test imager", scope, angle.Arcsec(1.5), 0.25)
	if err != nil {
		t.Fatalf("NewInstrument: %v", err)
	}

	inst.ReadNoiseElectrons = 3.0
	inst.DarkCurrentEPerSec = 0.01

	depth, err := sbplan.NewImaging(sbplan.Spec{
		Sky:            sky,
		Site:           site,
		Air:            atmosphere.RuralAerosol(site.Height(), atmosphere.CleanMountainAOD550),
		Instrument:     inst,
		Exposure:       300 * gotime.Second,
		AperturePixels: 9,
		SNR:            5,
	})
	if err != nil {
		t.Fatalf("NewImaging: %v", err)
	}

	when := time.Date(2026, gotime.March, 20, 5, 0, 0, 0, time.LocationUTC)

	zenith, err := depth.LimitingMagnitudeAt(when, angle.Deg(90), angle.Deg(0))
	if err != nil {
		t.Fatalf("LimitingMagnitudeAt: %v", err)
	}

	low, err := depth.LimitingMagnitudeAt(when, angle.Deg(10), angle.Deg(0))
	if err != nil {
		t.Fatalf("LimitingMagnitudeAt: %v", err)
	}

	t.Logf("300 s, SNR 5, 9 px, %s band: zenith %.2f, 10 degrees %.2f",
		depth.Band(), zenith, low)

	// A 20 cm telescope in five minutes reaches somewhere in the high teens
	// to low twenties. The bound is wide on purpose: it is here to catch a
	// units error of several magnitudes, not to pin a number this test has no
	// independent source for.
	if zenith < 15 || zenith > 26 {
		t.Errorf("the zenith limit is %.2f, outside anything a 200 mm telescope reaches in "+
			"five minutes — that is a unit error rather than a modelling difference", zenith)
	}

	// Shallower down low, but by less than the V-band sky brightens: with no
	// filter declared the electrons are broadband and the redder half of the
	// grid is extinguished less. Asserting only the ordering, because the
	// size of the gap is a property of the throughput this test declines to
	// specify.
	if low >= zenith {
		t.Errorf("ten degrees reaches %.2f against the zenith's %.2f; the sky is brighter "+
			"down there, so the limit must be shallower", low, zenith)
	}
}

// The depth model drops into a planning constraint without adapters.
//
// The point of the interface being one method wide: nothing in plan knows
// what produced the number, and nothing here knows what plan does with it.
func TestImagingSatisfiesThePlanningConstraint(t *testing.T) {
	ids, size := dataset.Endpoints(skybrightness.GAMBONSWeb)
	remote.EnableDownloads(size, ids...)

	site, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	sky, err := dataset.Open(context.Background(), dataset.Spec{Preset: skybrightness.GAMBONSWeb})
	if err != nil {
		t.Skipf("no sky available: %v", err)
	}

	scope, err := optics.NewTelescope(200, 1000)
	if err != nil {
		t.Fatalf("NewTelescope: %v", err)
	}

	inst, err := optics.NewInstrument("test imager", scope, angle.Arcsec(1.5), 0.25)
	if err != nil {
		t.Fatalf("NewInstrument: %v", err)
	}

	inst.ReadNoiseElectrons = 3.0

	depth, err := sbplan.NewImaging(sbplan.Spec{
		Sky: sky, Site: site, Instrument: inst,
		Exposure: 300 * gotime.Second, AperturePixels: 9, SNR: 5,
	})
	if err != nil {
		t.Fatalf("NewImaging: %v", err)
	}

	planSite, err := plan.NewSite("Paranal", site)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	when := time.Date(2026, gotime.March, 20, 5, 0, 0, 0, time.LocationUTC)

	// A bright star, which any such instrument reaches, and an absurdly faint
	// one, which none does. Both are above the horizon at this instant.
	bright := plan.NewStar("bright", angle.Hour(16.5), angle.Deg(-24.6),
		plan.WithStarMagnitude(8))
	faint := plan.NewStar("faint", angle.Hour(16.5), angle.Deg(-24.6),
		plan.WithStarMagnitude(35))

	c := plan.LimitingMagnitudeConstraint{Sky: depth, Boolean: true}

	got, err := c.Check(bright, when, planSite)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	if !got.Pass {
		t.Errorf("a magnitude-8 star was rejected: %v", got)
	}

	got, err = c.Check(faint, when, planSite)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	if got.Pass {
		t.Errorf("a magnitude-35 star passed: %v", got)
	}
}
