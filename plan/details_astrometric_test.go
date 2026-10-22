package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// TestDetailsRADecAreTheAstrometricPlaceFromTheSite: TargetDetails' RA and
// Dec are what their doc says, the astrometric place seen from the site
// (#843), bit for bit what the ephemeris gives for the Site observer, whose
// own tests hold it to SOFA. The Elongation is measured to the Sun placed the
// same way. A MovingBody written elsewhere keeps its own GeocentricVec, less
// the site.
//
// The value published before, the geocentric apparent place less the site,
// carries the annual aberration: 20.5″ times the sine of the angle from the
// Earth's apex, up to 21.0″ across 2026. At this instant that is 10.5″ for
// the Moon and 17.0″ for Mars, and the last check of each holds the gap above
// 1″, so a return to it cannot pass.
func TestDetailsRADecAreTheAstrometricPlaceFromTheSite(t *testing.T) {
	t.Parallel()

	p := eph.Default()

	site, err := coord.NewEarthLocation(41.3874, 2.1686, 0)
	if err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, time.June, 10, 21, 0, 0, 0, time.LocationUTC)
	ctx := coord.NewContext(at, site, atmosphere.Refraction{})

	direction := func(v vector.Vec3) coord.ICRS {
		ra, dec := v.ToSpherical()

		return coord.NewICRS(angle.Rad(ra), angle.Rad(dec))
	}

	for _, b := range []*Planet{NewMoon(p), NewPlanet("Mars", eph.MarsBarycenter, p), NewSun(p)} {
		d, err := b.GetDetails(ctx, DetailOverrides{})
		if err != nil {
			t.Fatalf("%s: %v", b.name, err)
		}

		in := eph.At(p, at)

		st, err := in.Astrometric(b.id, eph.Site(ctx))
		if err != nil {
			t.Fatalf("%s: %v", b.name, err)
		}

		want := direction(st.Pos)
		if d.RA != want.RA() || d.Dec != want.Dec() {
			t.Errorf("%s: details RA %v, Dec %v; the astrometric place from the site is RA %v, Dec %v",
				b.name, d.RA, d.Dec, want.RA(), want.Dec())
		}

		if b.IsSun() {
			continue
		}

		sun, err := in.Astrometric(eph.Sun, eph.Site(ctx))
		if err != nil {
			t.Fatalf("%s: the Sun: %v", b.name, err)
		}

		if elong := coord.Separation(want, direction(sun.Pos)); d.Elongation != elong {
			t.Errorf("%s: elongation %v, between the two astrometric places from the site %v",
				b.name, d.Elongation, elong)
		}

		geo, err := b.GeocentricVec(at)
		if err != nil {
			t.Fatalf("%s: %v", b.name, err)
		}

		if gap := coord.Separation(want, direction(geo.Sub(ctx.ObsVec()))).Arcsec(); gap < 1 {
			t.Errorf("%s: the geocentric apparent place less the site is only %.2f″ from the "+
				"astrometric place, so this test does not tell them apart", b.name, gap)
		}
	}

	moon := NewMoon(p)

	var d TargetDetails

	fillMovingBody(&d, customBody{moon}, at, ctx)

	geo, err := moon.GeocentricVec(at)
	if err != nil {
		t.Fatal(err)
	}

	if want := direction(geo.Sub(ctx.ObsVec())); d.RA != want.RA() || d.Dec != want.Dec() {
		t.Errorf("a caller's MovingBody: details RA %v, Dec %v; its GeocentricVec less the site, RA %v, Dec %v",
			d.RA, d.Dec, want.RA(), want.Dec())
	}
}
