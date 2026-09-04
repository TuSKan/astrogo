package plan_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// The ISS, so a satellite's altitude is exercised offline. Its own package's
// test elements, reused rather than fetched.
const (
	issLine1 = "1 25544U 98067A   26109.48995873  .00010082  00000-0  19194-3 0  9999"
	issLine2 = "2 25544  51.6329 230.6068 0006631 325.6576  34.3983 15.48833250562656"
)

// TestSatelliteAltitudePipelinesAgree extends the constraint-versus-details
// comparison to the body class with the most parallax of all.
//
// [TestConstraintAndDetailsAgreeOnAltitude] covers the Sun, Moon and two
// planets. A satellite is the extreme case and the one a star-shaped test is
// least like: the ISS orbits at about 400 km, so an observer's offset from the
// geocentre is a large fraction of its whole distance, where for the Moon it is
// 1/60th and for Mars nothing. If the topocentric dispatch were ever removed
// again, this is the case that would show it most violently.
func TestSatelliteAltitudePipelinesAgree(t *testing.T) {
	t.Parallel()

	site, err := plan.NewSiteEarthLocation("pipelines", -23.5, -46.6, 800)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	prov, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	iss := plan.NewSatellite("ISS", eph.ID(0), prov)

	const toleranceArcsec = 1.0

	// Two orbits at ten-minute steps, so the comparison spans a full sweep of
	// geometry rather than one point on one pass.
	for minute := 0; minute < 180; minute += 10 {
		when := time.Date(2026, time.April, 19, 12, minute, 0, 0, time.LocationUTC)
		ctx := coord.NewContext(when, site.Location(), site.Refraction())

		eval, err := plan.IsObservable(iss, when, site, plan.Altitude{Threshold: angle.Deg(0)})
		if err != nil {
			t.Fatalf("+%dmin IsObservable: %v", minute, err)
		}

		details, err := iss.GetDetails(ctx)
		if err != nil {
			t.Fatalf("+%dmin GetDetails: %v", minute, err)
		}

		if d := math.Abs(eval.AltAz.Alt().Degrees()-details.Altitude.Degrees()) * 3600; d > toleranceArcsec {
			t.Errorf("+%dmin: IsObservable reports %.4f° and GetDetails %.4f°, a difference "+
				"of %.1f arcsec.\n  A satellite is where a geocentric shortcut shows up "+
				"worst — see observedAltAz.",
				minute, eval.AltAz.Alt().Degrees(), details.Altitude.Degrees(), d)
		}
	}
}

// TestEventAltitudeConventionIsWhatItClaims pins the third pipeline against the
// other two, and the answer is more interesting than "they agree".
//
// # What comparing them found
//
// At transit, Event.Altitude matches IsObservable and GetDetails exactly. At
// rise and set it differs by 0.1668°, at both ends, for the Moon:
//
//	Rise       Event= -1.6547  IsObservable= -1.4879  GetDetails= -1.4879
//	Transit    Event= 54.2489  IsObservable= 54.2489  GetDetails= 54.2489
//	Set        Event= -1.6547  IsObservable= -1.4879  GetDetails= -1.4879
//
// That 0.1668° is refraction — the same value coord's own refraction tests
// measure at these altitudes. A constant offset at both ends, and none at
// transit, rules out a timing difference and identifies the cause exactly.
//
// It is deliberate. The solver builds the rise/set context with a
// no-refraction atmosphere (events.go, "using the same no-refraction
// atmosphere as the solver") because rise and set are solved against a
// geometric threshold plus a constant horizon allowance, the USNO convention.
// Transit is built with the site's real refraction.
//
// So Event.Altitude means different things for different event kinds, and
// nothing said so. Asserting a three-way equality would have been asserting
// something false; this asserts the convention instead, which is the useful
// thing to hold still.
//
// # Why this belongs with the others
//
// #100 and #101 were both two public APIs computing one quantity by different
// routes and disagreeing, with no test looking. This is the third route. It
// does not disagree — it answers a different question — but until something
// compared them, nobody could tell those apart, which is the same gap.
func TestEventAltitudeConventionIsWhatItClaims(t *testing.T) {
	t.Parallel()

	site, err := plan.NewSiteEarthLocation("pipelines", -23.5, -46.6, 800)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	prov := eph.Default()
	moon := plan.NewMoon(prov)

	start := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(2026, time.March, 21, 0, 0, 0, 0, time.LocationUTC)

	events, err := plan.MoonEvents(start, end, site, prov)
	if err != nil {
		t.Fatalf("MoonEvents: %v", err)
	}

	if len(events) < 3 {
		t.Fatalf("got %d events over a day, expected rise, transit and set", len(events))
	}

	const toleranceArcsec = 1.0

	var checked int

	for _, e := range events {
		// The same instant through both conventions, so the comparison does
		// not depend on which one the event happens to use.
		refracted := coord.NewContext(e.Time, site.Location(), site.Refraction())
		geometric := coord.NewContext(e.Time, site.Location(), atmosphere.Refraction{})

		vec, err := moon.GeocentricVec(e.Time)
		if err != nil {
			t.Fatalf("%s GeocentricVec: %v", e.Kind, err)
		}

		withRefraction := refracted.GeocentricToObserved(vec).Alt().Degrees()
		withoutRefraction := geometric.GeocentricToObserved(vec).Alt().Degrees()

		var (
			want float64
			why  string
		)

		if e.Kind == plan.EventTransit {
			want, why = withRefraction, "transit is solved with the site's own refraction"
		} else {
			want, why = withoutRefraction, "rise and set are solved against a geometric "+
				"threshold with a constant horizon allowance, so the reported altitude "+
				"carries no refraction"
		}

		checked++

		if d := math.Abs(e.Altitude.Degrees()-want) * 3600; d > toleranceArcsec {
			t.Errorf("%s: Event.Altitude is %.4f°, expected %.4f° (%.1f arcsec out).\n"+
				"  %s.\n  Refracted here is %.4f° and geometric %.4f°; if the event now "+
				"matches the other one, the convention changed and this test is the "+
				"record of what it used to be.",
				e.Kind, e.Altitude.Degrees(), want, d, why, withRefraction, withoutRefraction)
		}

		// Altitude and GeometricAltitude are assigned the same value at both
		// construction sites, so they carry one quantity under two names. That
		// is worth knowing about rather than relying on.
		if e.Altitude != e.GeometricAltitude {
			t.Errorf("%s: Altitude %v and GeometricAltitude %v differ; they have always "+
				"been assigned the same value, so something now distinguishes them and "+
				"the convention above needs revisiting",
				e.Kind, e.Altitude, e.GeometricAltitude)
		}
	}

	if checked < 3 {
		t.Fatalf("only %d events checked", checked)
	}

	t.Logf("%d events checked against their own altitude convention", checked)
}
