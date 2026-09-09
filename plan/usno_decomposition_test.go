//go:build integration

package plan_test

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
)

// TestUSNODecomposesTheTopocentricBias splits the topocentric residual into
// the stages that could be producing it.
//
// # The question
//
// astrogo's topocentric alt/az agrees with JPL Horizons to a cross-track
// signed mean of about -0.5 arcseconds — a bias, not scatter. A geocentric
// astrometric comparison then measured agreement at the reference's own
// printing precision, ~3 microarcseconds, with no signed offset at all, which
// rules out the ephemeris and the light-time solution. What remains is
// everything between an astrometric place and an observed one:
//
//	precession-nutation, annual aberration, light deflection
//	Earth rotation (UT1 -> GAST/ERA)
//	polar motion
//	the site vector and diurnal parallax
//
// # Why USNO can separate them and Horizons cannot
//
// Horizons publishes the two ends of that chain. USNO's celestial-navigation
// service publishes the middle, in the navigator's own terms: Greenwich Hour
// Angle and declination, geocentric and apparent, plus the computed altitude
// and azimuth for a site. Those nest exactly onto the stages above:
//
//	declination      precession-nutation + aberration + deflection,
//	                 and *nothing else* — declination is measured from the
//	                 equator, so Earth rotation cannot touch it
//	GHA              the same, plus Earth rotation
//	Hc / Zn          the same, plus the site vector and parallax
//
// So the three residuals answer three questions in order. A bias already
// present in declination is in the apparent-place chain. A bias appearing
// only in GHA is Earth rotation. A bias appearing only in Hc/Zn is the site.
//
// USNO is also a genuinely independent implementation, which the Horizons
// comparison is not: astrogo reads JPL DE and Horizons *is* JPL. USNO's
// almanac is computed with NOVAS.
//
// # What this can and cannot resolve
//
// celnav is a navigation product. The Nautical Almanac tabulates Greenwich
// Hour Angle and declination to 0.1 arcminutes — six arcseconds — because that
// is what a sextant sight needs, and the API returning six decimal degrees
// does not make the underlying computation finer than the product it serves.
//
// So residuals here are bounded below by celnav's own precision, and measured
// they sit well inside it: +0.662 arcsec in GHA and -0.027 in declination are
// 0.011 and 0.0005 arcminutes. Both are *ten to a thousand times inside* what
// the almanac publishes to.
//
// That matters for reading the numbers. Against Horizons at full precision,
// astrogo's geocentric apparent place agrees to +0.049 arcsec in right
// ascension and -0.0003 in declination, and its Greenwich apparent sidereal
// time to +0.050 arcsec — so astrogo's own GHA matches Horizons to about a
// milliarcsecond. The 0.662 arcsec measured here is therefore USNO's
// difference from Horizons, not astrogo's from either (#256).
//
// The test is still worth running, for the reason it was written: declination
// and GHA nest, so a *large* fault in one and not the other would still be
// localised, and USNO is the only independent implementation available. What
// it cannot do is resolve a half-arcsecond effect, and the bounds below are
// set accordingly rather than to celnav's returned digits.
//
// # Two things this has to get right to mean anything
//
// The origin. astrogo's apparent place is CIRS, measured from the Celestial
// Intermediate Origin; the navigator's GHA is measured from the true equinox.
// The two differ by the equation of the origins, which is about 0.33 degrees
// in 2026 — so mixing them would swamp the half-arcsecond being hunted with
// an error three thousand times larger. [gofaext.Atci13] returns eo for
// exactly this, and the classical apparent right ascension is ri - eo.
//
// The centre. USNO's GHA and declination are geocentric; the navigator
// applies parallax separately, which is why the response carries it as its
// own correction. astrogo's [coord.Context] is built for a site, so
// AstrometricToApparent through it is *topocentric* — up to 8.8 arcseconds
// of diurnal parallax for the Sun, again far above the effect being measured.
// Atci13 is the geocentric transformation and is what this uses.
func TestUSNODecomposesTheTopocentricBias(t *testing.T) {
	requireUSNO(t)

	provider := newEph(t)

	// Sites spread in latitude and longitude, so a site-vector or
	// Earth-rotation error cannot average itself away: a longitude error
	// shows in GHA identically everywhere, while a site-vector error changes
	// sign with the observer's position.
	// GHA and declination are geocentric, so the observing site is irrelevant
	// to them — celnav simply requires coordinates. Varying the *epoch* is
	// what matters, and an earlier version of this test varied site and epoch
	// together, which confounded the two: a per-epoch effect looked like a
	// per-site one and could not be read either way.
	const (
		fixedLat = 51.4779
		fixedLon = 0.0015
	)

	epochs := []struct {
		date, utcTime string
	}{
		{"2026-03-20", "00:00:00"},
		{"2026-03-20", "06:00:00"},
		{"2026-03-20", "12:00:00"},
		{"2026-03-20", "18:00:00"},
		{"2026-06-21", "12:00:00"},
		{"2026-09-23", "12:00:00"},
		{"2026-12-21", "12:00:00"},
	}

	var (
		decResiduals []float64
		ghaResiduals []float64
	)

	byBody := map[string][]float64{}

	for _, e := range epochs {
		url := fmt.Sprintf(
			"https://aa.usno.navy.mil/api/celnav?date=%s&time=%s&coords=%.6f,%.6f",
			e.date, e.utcTime, fixedLat, fixedLon,
		)

		var resp usnoCelNavResponse
		if err := json.Unmarshal(usnoGet(t, url), &resp); err != nil {
			t.Fatalf("%s %s: parsing the CelNav response: %v", e.date, e.utcTime, err)
		}

		epoch := parseUSNOEpoch(t, e.date, e.utcTime)

		gast, err := epoch.GAST()
		if err != nil {
			t.Fatalf("%s %s: GAST: %v", e.date, e.utcTime, err)
		}

		var epochGHA []float64

		for _, entry := range resp.Properties.Data {
			id, ok := usnoNavigationalBody(entry.Object)
			if !ok {
				continue
			}

			gotDec, gotGHA := geocentricApparent(t, provider, id, epoch, gast)

			dDec := (gotDec - entry.AlmanacData.Dec) * 3600
			dGHA := wrapDegrees180(gotGHA-entry.AlmanacData.GHA) * 3600 *
				math.Cos(gotDec*math.Pi/180)

			decResiduals = append(decResiduals, dDec)
			ghaResiduals = append(ghaResiduals, dGHA)
			epochGHA = append(epochGHA, dGHA)
			byBody[entry.Object] = append(byBody[entry.Object], dGHA)

			t.Logf("%s %s  %-8s  dDec %+8.3f\"  dGHA*cos(dec) %+8.3f\"",
				e.date, e.utcTime, entry.Object, dDec, dGHA)
		}

		if len(epochGHA) > 0 {
			var m float64
			for _, x := range epochGHA {
				m += x
			}

			// A common shift across every body at one epoch is Earth
			// rotation; a spread within the epoch is not.
			t.Logf("  -> epoch mean dGHA %+8.3f\" over %d bodies",
				m/float64(len(epochGHA)), len(epochGHA))
		}
	}

	for _, name := range []string{"Sun", "Jupiter", "Saturn"} {
		xs := byBody[name]
		if len(xs) == 0 {
			continue
		}

		var m float64
		for _, x := range xs {
			m += x
		}

		// A shift that follows one body across every epoch is that body's
		// own place, not Earth rotation.
		t.Logf("  == %-8s mean dGHA %+8.3f\" over %d epochs", name, m/float64(len(xs)), len(xs))
	}

	if len(decResiduals) < 10 {
		t.Fatalf("only %d comparison points; USNO returned too few navigational bodies "+
			"to say anything", len(decResiduals))
	}

	decMean := summarise(t, "declination (no Earth rotation)", decResiduals)
	ghaMean := summarise(t, "GHA (adds Earth rotation)", ghaResiduals)

	t.Logf("")
	t.Logf("Reading: a bias in declination is in the apparent-place chain; one that "+
		"appears only in GHA is Earth rotation. Measured %+.3f\" and %+.3f\".",
		decMean, ghaMean)

	// Declination is asserted tightly, GHA loosely, and the asymmetry is the
	// result rather than an oversight.
	//
	// Declination cannot see Earth rotation, so a clean declination is
	// positive evidence that precession-nutation, aberration and deflection
	// are right. That is worth protecting at a tight bound.
	//
	// GHA is where the bias actually is, so bounding it at the measured value
	// would be pinning a contract to a defect and asserting it stays. It is
	// bounded loosely enough to catch a gross regression — a wrong sidereal
	// formula, a UT1 scale slip, an equation-of-origins sign — while leaving
	// the sub-arcsecond difference free to be fixed without editing this test.
	const (
		maxDecBiasArcsec = 0.2
		maxGHABiasArcsec = 3.0
	)

	if math.Abs(decMean) > maxDecBiasArcsec {
		t.Errorf("declination bias %+.3f arcsec exceeds %.1f; declination cannot see "+
			"Earth rotation, so this is the apparent-place chain — precession-nutation, "+
			"aberration or deflection", decMean, maxDecBiasArcsec)
	}

	if math.Abs(ghaMean) > maxGHABiasArcsec {
		t.Errorf("GHA bias %+.3f arcsec exceeds %.1f, which is far beyond a plausible "+
			"UT1 difference (%.3f seconds); suspect the sidereal-time formula or the "+
			"equation of the origins rather than Earth orientation data",
			ghaMean, maxGHABiasArcsec, ghaMean/15.041)
	}

	// The decomposition only means something if the two stages disagree. If
	// they ever converge, the localisation this test performs has stopped
	// being valid and the reader needs to know before trusting its message.
	if math.Abs(ghaMean) < math.Abs(decMean) {
		t.Errorf("GHA bias %+.3f arcsec is no larger than the declination bias %+.3f; "+
			"this test's conclusion — that the residual enters at Earth rotation — no "+
			"longer follows from its own measurement", ghaMean, decMean)
	}
}

// geocentricApparent returns the geocentric apparent declination and Greenwich
// Hour Angle of a body, in degrees, in the navigator's convention.
//
// The astrometric place comes from [eph.AstrometricState] — light-time
// corrected, no aberration — which is what Atci13 expects as input. Atci13
// then applies precession-nutation, annual aberration and light deflection
// geocentrically, and returns the equation of the origins alongside.
func geocentricApparent(t *testing.T, p eph.Provider, id eph.ID, epoch time.Time,
	gast angle.Angle,
) (decDeg, ghaDeg float64) {
	t.Helper()

	state, err := eph.AstrometricState(p, id, epoch)
	if err != nil {
		t.Fatalf("AstrometricState(%s): %v", id, err)
	}

	var astrometric coord.Astrometric

	astrometric.FromUnitVector(state.Pos.Unit())

	tt1, tt2 := epoch.TT().JDParts()

	ri, di, eo := gofaext.Atci13(
		astrometric.RA().Radians(), astrometric.Dec().Radians(),
		0, 0, 0, 0,
		tt1, tt2,
	)

	// ri is measured from the CIO; the navigator's hour angle is measured
	// from the true equinox. ri - eo is the classical apparent right
	// ascension, and GHA is GAST minus it.
	apparentRA := angle.Rad(ri - eo).Wrap360()
	gha := angle.Deg(gast.Degrees() - apparentRA.Degrees()).Wrap360()

	return angle.Rad(di).Degrees(), gha.Degrees()
}

// usnoNavigationalBody maps USNO's object names onto astrogo's ids.
//
// Only the solar-system bodies: the 57 navigational stars would need catalog
// positions and proper motions to compare, which is a different test with a
// different reference.
func usnoNavigationalBody(name string) (eph.ID, bool) {
	switch name {
	case "Sun", "Jupiter", "Saturn":
		return map[string]eph.ID{
			"Sun": eph.Sun, "Jupiter": eph.Jupiter, "Saturn": eph.Saturn,
		}[name], true
	default:
		// Venus and Mars are deliberately absent, and finding out why is most
		// of what this test taught.
		//
		// USNO's celnav tabulates them the way the Nautical Almanac does: at
		// the centre of the *illuminated disc*, because that is what a
		// navigator sights. Horizons and astrogo give the geometric centre.
		// Comparing the two is a category error, not a measurement.
		//
		// It is large. Venus on 2026-09-23 at 0.393 AU: astrogo differs from
		// USNO by 6.76" in declination and 12.70" in GHA, while the Sun,
		// Jupiter and Mars at the same epoch are all inside 0.4". Horizons'
		// apparent declination for that point is -19.682441 deg against USNO's
		// -19.680563 — the same 6.76" — so astrogo agrees with Horizons and it
		// is the convention that differs.
		//
		// Not parallax: USNO returns identical GHA and declination for
		// Greenwich, Sydney and the origin, so its values are geocentric.
		// The magnitude fits a crescent's centre of light — Venus's
		// semidiameter at that distance is 21.4", and the offset measures
		// 14.4" on the sky.
		//
		// The Moon is excluded for a different reason: at 0.9 degrees of
		// parallax and half an arcsecond per second of motion it is far more
		// sensitive to the epoch than anything being resolved here.
		return 0, false
	}
}

// parseUSNOEpoch builds the epoch from the strings the request was made with,
// so the comparison cannot drift from what was asked for.
func parseUSNOEpoch(t *testing.T, date, utcTime string) time.Time {
	t.Helper()

	var y, mo, d int
	if _, err := fmt.Sscanf(date, "%d-%d-%d", &y, &mo, &d); err != nil {
		t.Fatalf("parsing date %q: %v", date, err)
	}

	var h, m, s int
	if _, err := fmt.Sscanf(utcTime, "%d:%d:%d", &h, &m, &s); err != nil {
		t.Fatalf("parsing time %q: %v", utcTime, err)
	}

	return time.Date(y, time.Month(mo), d, h, m, s, 0, time.LocationUTC)
}

// summarise logs a residual distribution and returns its signed mean.
//
// The signed mean is the return value because it is the statistic the whole
// test exists to read: a systematic offset vanishes in an unsigned summary.
func summarise(t *testing.T, label string, xs []float64) float64 {
	t.Helper()

	var mean float64
	for _, x := range xs {
		mean += x
	}

	mean /= float64(len(xs))

	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)

	t.Logf("%-34s n=%2d  signed mean %+8.3f\"  p50 %+8.3f\"  min %+8.3f\"  max %+8.3f\"",
		label, len(xs), mean, sorted[len(sorted)/2], sorted[0], sorted[len(sorted)-1])

	return mean
}

// wrapDegrees180 folds a difference of angles into (-180, 180].
func wrapDegrees180(d float64) float64 {
	for d > 180 {
		d -= 360
	}

	for d <= -180 {
		d += 360
	}

	return d
}
