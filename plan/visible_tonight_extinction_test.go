package plan_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/unit"
)

// defaultNight is the air WithAtmosphere documents as VisibleTonight's
// default at a site's height: the standard atmosphere's pressure there, 258 DU
// of ozone, and Paranal's median aerosol under OPAC's continental clean type.
func defaultNight(t *testing.T, height unit.Length) *atmosphere.Atmosphere {
	t.Helper()

	air, err := atmosphere.ContinentalCleanAerosol(height, atmosphere.CleanMountainAOD550).Ozone(258).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	return air
}

// extinctionV is air's extinction coefficient at V's pivot, 547.8 nm.
func extinctionV(t *testing.T, air *atmosphere.Atmosphere) float64 {
	t.Helper()

	k, err := air.Extinction(547.8)
	if err != nil {
		t.Fatalf("Extinction: %v", err)
	}

	return k
}

// dimming returns how many magnitudes VisibleTonight dimmed obj by, and how
// many air dims V arriving from obj's peak, or from the horizon for a peak
// below it.
func dimming(t *testing.T, obj plan.VisibleObject, air *atmosphere.Atmosphere) (got, want float64) {
	t.Helper()

	peak := obj.PeakAltitude
	if peak.Degrees() < 0 {
		peak = angle.Zero()
	}

	want, err := air.ExtinctionToward(547.8, peak)
	if err != nil {
		t.Fatalf("ExtinctionToward: %v", err)
	}

	return obj.ApparentMag - obj.Target.VMag, want
}

// siriusTonight runs VisibleTonight for Sirius alone and returns it.
func siriusTonight(t *testing.T, site *plan.Site, opts ...plan.VisibleTonightOption) plan.VisibleObject {
	t.Helper()

	sources := []resolve.BrightObjectSearcher{&mockBrightSource{targets: []resolve.Target{sirius}}}

	results, err := plan.VisibleTonight(context.Background(), site, testNight, 2, sources, ephemeris.Default(), opts...)
	if err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	got, ok := findByName(results, "Sirius")
	if !ok {
		t.Fatalf("Sirius was not listed at %s", site.Name())
	}

	return got
}

// VisibleTonight dims every object by the air it is seen through: by default
// a clean night at the site's own height, and otherwise whatever air the
// caller names. A nil air is the default, not no air at all.
func TestVisibleTonightDimsThroughTheSitesAir(t *testing.T) {
	t.Parallel()

	site := quintaCalixtoSite(t)

	hazy, err := atmosphere.RuralAerosol(site.Height(), atmosphere.UrbanAOD550).Ozone(320).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	cases := []struct {
		name string
		opts []plan.VisibleTonightOption
		air  *atmosphere.Atmosphere
	}{
		{"default", nil, defaultNight(t, site.Height())},
		{"named air", []plan.VisibleTonightOption{plan.WithAtmosphere(hazy)}, hazy},
		{"nil air", []plan.VisibleTonightOption{plan.WithAtmosphere(nil)}, defaultNight(t, site.Height())},
	}

	for _, c := range cases {
		got, want := dimming(t, siriusTonight(t, site, c.opts...), c.air)
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: Sirius was dimmed by %.6f mag, want the air's %.6f toward its peak at V", c.name, got, want)
		}

		t.Logf("%-9s dimmed by %.4f mag, k(V) = %.4f", c.name, got, extinctionV(t, c.air))
	}
}

// The default is a clean night at the site's own height, so it falls with the
// air above the site, and at Paranal it is the air Patat et al. (2011)
// measured: their aerosol and ozone, with the standard atmosphere's pressure
// in place of their barometer's. Its coefficient reproduces the extinction
// they measured around 550 nm, 0.131 and 0.129 mag per airmass at 547.5 and
// 552.5 nm, within the 0.01 they state for their curve, and VisibleTonight
// dims Sirius through that air at each site.
//
// The figures WithAtmosphere quotes, 0.162 at sea level and 0.132 at Paranal,
// are held here to the three decimals it prints them to.
func TestVisibleTonightDefaultAirFollowsTheSite(t *testing.T) {
	t.Parallel()

	seaLevel, err := plan.NewSiteEarthLocation("sea level", -22.528478, -46.473002, 0)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	paranal, err := plan.NewSiteEarthLocation("Paranal", -24.6272, -70.4045, 2640)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	for _, site := range []*plan.Site{seaLevel, paranal} {
		got, want := dimming(t, siriusTonight(t, site), defaultNight(t, site.Height()))
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: Sirius was dimmed by %.6f mag, want the default air's %.6f", site.Name(), got, want)
		}
	}

	kSea := extinctionV(t, defaultNight(t, seaLevel.Height()))
	kParanal := extinctionV(t, defaultNight(t, paranal.Height()))

	if math.Abs(kSea-0.162) > 0.0005 {
		t.Errorf("at sea level the default dims V by %.4f mag per airmass; WithAtmosphere says 0.162", kSea)
	}

	if math.Abs(kParanal-0.132) > 0.0005 {
		t.Errorf("at Paranal the default dims V by %.4f mag per airmass; WithAtmosphere says 0.132", kParanal)
	}

	const patat550 = (0.131 + 0.129) / 2 // Table B.1 at 547.5 and 552.5 nm
	if math.Abs(kParanal-patat550) > 0.01 {
		t.Errorf("at Paranal the default dims V by %.4f mag per airmass; Patat et al. measured %.3f",
			kParanal, patat550)
	}

	t.Logf("default k(V): sea level %.4f, Paranal %.4f (Patat et al. %.3f)", kSea, kParanal, patat550)
}

// An air whose extinction cannot be computed fails the call rather than
// passing every object through undimmed. The zero Atmosphere has no surface
// pressure.
func TestVisibleTonightRefusesAnAirWithNoPressure(t *testing.T) {
	t.Parallel()

	sources := []resolve.BrightObjectSearcher{&mockBrightSource{targets: []resolve.Target{sirius}}}

	_, err := plan.VisibleTonight(context.Background(), quintaCalixtoSite(t), testNight, 2, sources,
		ephemeris.Default(), plan.WithAtmosphere(&atmosphere.Atmosphere{}))
	if !errors.Is(err, atmosphere.ErrPressure) {
		t.Fatalf("VisibleTonight with the zero Atmosphere = %v, want ErrPressure", err)
	}
}
