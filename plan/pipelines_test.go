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

// TestEventAltitudesAreWhatTheyAreNamed pins the third pipeline against the
// other two, and the answer was more interesting than "they agree".
//
// Named TestEventAltitudeConventionIsWhatItClaims until #156 — since renamed,
// because it no longer records a convention that had to be discovered by
// measurement; it checks the field names are true.
//
// # What comparing them found
//
// At transit, Event.Altitude matched IsObservable and GetDetails exactly. At
// rise and set it differed by 0.1668 degrees, at both ends, for the Moon:
//
//	Rise       Event= -1.6547  IsObservable= -1.4879  GetDetails= -1.4879
//	Transit    Event= 54.2489  IsObservable= 54.2489  GetDetails= 54.2489
//	Set        Event= -1.6547  IsObservable= -1.4879  GetDetails= -1.4879
//
// That 0.1668 degrees is refraction. A constant offset at both ends and none
// at transit ruled out a timing difference and identified the cause exactly:
// the solver built the rise/set context with a no-refraction atmosphere,
// because rise and set are solved against a geometric threshold with the
// horizon allowance already folded in, the USNO convention. Transit was built
// with the site's real refraction.
//
// So Event.Altitude meant different things for different event kinds, and
// nothing said so. Event.GeometricAltitude was assigned the identical value at
// both sites, so it distinguished nothing.
//
// # What this now asserts
//
// Both fields are populated honestly at both construction sites, so the test
// no longer records a convention — it checks the names are true:
//
//   - Altitude is the refracted altitude, at every event kind, which makes it
//     equal to what IsObservable and GetDetails report. Comparing a rise
//     altitude against a transit altitude is now sound.
//   - GeometricAltitude is the geometric one, at every event kind.
//   - The gap between them is the refraction at that altitude, not zero.
//   - Value is still measured geometrically at rise and set, because that is
//     what the threshold means. Changing the reported altitude must not move
//     the event times, and this is what says so.
func TestEventAltitudesAreWhatTheyAreNamed(t *testing.T) {
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

	var checked, sawRiseOrSet, sawTransit int

	for _, e := range events {
		refracted := coord.NewContext(e.Time, site.Location(), site.Refraction())
		geometric := coord.NewContext(e.Time, site.Location(), atmosphere.Refraction{})

		vec, err := moon.GeocentricVec(e.Time)
		if err != nil {
			t.Fatalf("%s GeocentricVec: %v", e.Kind, err)
		}

		withRefraction := refracted.GeocentricToObserved(vec).Alt().Degrees()
		withoutRefraction := geometric.GeocentricToObserved(vec).Alt().Degrees()

		checked++

		if d := math.Abs(e.Altitude.Degrees()-withRefraction) * 3600; d > toleranceArcsec {
			t.Errorf("%s: Altitude is %.4f deg, want the refracted %.4f deg "+
				"(%.1f arcsec out). Altitude is what an observer sees, at every "+
				"event kind, so it must agree with IsObservable and GetDetails.",
				e.Kind, e.Altitude.Degrees(), withRefraction, d)
		}

		if d := math.Abs(e.GeometricAltitude.Degrees()-withoutRefraction) * 3600; d > toleranceArcsec {
			t.Errorf("%s: GeometricAltitude is %.4f deg, want the unrefracted %.4f deg "+
				"(%.1f arcsec out).", e.Kind, e.GeometricAltitude.Degrees(),
				withoutRefraction, d)
		}

		// The two fields must actually differ, by the refraction at this
		// altitude. Equal fields are the defect this test exists for: they
		// were assigned the same value at both sites.
		gap := (e.Altitude - e.GeometricAltitude).Arcseconds()
		want := (withRefraction - withoutRefraction) * 3600

		if math.Abs(gap-want) > toleranceArcsec {
			t.Errorf("%s: Altitude and GeometricAltitude are %.2f arcsec apart, want "+
				"%.2f — the refraction at %.2f deg.", e.Kind, gap, want,
				e.GeometricAltitude.Degrees())
		}

		if gap <= 0 {
			t.Errorf("%s: the two altitudes are %.2f arcsec apart; refraction raises "+
				"an object, so the refracted one must be the higher", e.Kind, gap)
		}

		// Value keeps its geometric meaning at rise and set: it is the residual
		// against the threshold the solver actually used. If this moved, the
		// event times would have moved with it.
		switch e.Kind { //nolint:exhaustive // MoonEvents yields only rise, transit and set
		case plan.EventTransit:
			sawTransit++
		case plan.EventRise, plan.EventSet:
			sawRiseOrSet++

			residual := e.GeometricAltitude.Degrees() - site.MoonRiseSetThreshold().Degrees()
			if d := math.Abs(e.Value-residual) * 3600; d > toleranceArcsec {
				t.Errorf("%s: Value is %.6f but the geometric residual against the "+
					"threshold is %.6f (%.1f arcsec out). Value must stay geometric, "+
					"or the reported event times have shifted.", e.Kind, e.Value, residual, d)
			}
		default:
		}
	}

	if sawRiseOrSet == 0 || sawTransit == 0 {
		t.Fatalf("checked %d events but saw %d rise/set and %d transit; the test needs "+
			"both kinds to compare them", checked, sawRiseOrSet, sawTransit)
	}

	t.Logf("%d events checked (%d rise/set, %d transit)", checked, sawRiseOrSet, sawTransit)
}
