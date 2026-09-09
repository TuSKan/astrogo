package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// diurnalSites are the four latitudes the aberration is checked at: the
// equator, where the term is largest; two real observatories north and south;
// and a high latitude, where it nearly vanishes. The magnitude scales as the
// site's distance from the spin axis, so latitude is the variable that matters
// and longitude is only there to keep the sites distinguishable.
var diurnalSites = []struct {
	name              string
	lon, lat, heightM float64
}{
	{"Equator", 0, 0, 0},
	{"Paranal", -70.4042, -24.6272, 2635},
	{"Greenwich", 0, 51.4778, 46},
	{"High latitude 78N", 15.65, 78.22, 78},
}

// diurnalDirections spread eight CIRS directions around the sky so the
// eastward shift is sampled near its maximum, near zero, and at both signs. A
// single direction could sit where the aberration happens to project to
// nothing.
var diurnalDirections = []struct{ raHr, decDeg float64 }{
	{0, 0}, {3, 20}, {6, -30}, {9, 45}, {12, 0}, {15, -60}, {18, 30}, {21, 10},
}

// diurnalEpoch is fixed. A validation result whose epoch is "now" cannot be
// reproduced.
var diurnalEpoch = time.Date(2026, time.June, 21, 18, 42, 9, 0, time.LocationUTC)

// TestGeocentricToObservedAppliesDiurnalAberration is the regression test for
// #261, where the vector reduction route omitted the term entirely.
//
// # The defect
//
// Diurnal aberration is the observer's own rotation velocity — 465 m/s
// eastward at the equator, 0.32 arcseconds of displacement, falling with the
// site's distance from the spin axis. SOFA lets it enter at either of two
// places. Apco13 puts the observer's full barycentric velocity into ASTROM.V,
// so Atciq's aberration already carries it, and Apco13 correspondingly sets
// ASTROM.Diurab to zero; Apio13, which serves a CIRS-to-observed call with no
// Atciq in front of it, does the opposite and lets Atioq apply it.
//
// [coord.Context.GeocentricToObserved] is the second case and had neither. It
// reduced a geocentric place by rotation and translation alone, so the term
// was simply absent — while [coord.Context.AstrometricToObserved], on the
// stellar route, applied it. Two public functions, the same target, the same
// site, the same instant, up to 0.32 arcseconds apart.
//
// # Two assertions, because agreement alone would not be enough
//
// The first compares against SOFA's Atio13: a different entry point, with its
// ASTROM built by Apio13 rather than Apco13, so it is a real cross-check and
// not the same call twice.
//
// The second is oracle-free. It reconstructs the pre-fix arithmetic — the pure
// rotation the function used to be — and asserts that the displacement from it
// obeys the aberration law, |v|/c · sin(theta) towards the velocity, with |v|/c
// computed from first principles as omega times the site's distance from the
// rotation axis over c. Two implementations of one model can be wrong
// together; a physical law they both have to satisfy is what catches that.
func TestGeocentricToObservedAppliesDiurnalAberration(t *testing.T) {
	// Not parallel: the EOP model is process-wide.
	t.Cleanup(time.ResetEOP)

	const (
		dut1Seconds = 0.1237
		xpArcsec    = 0.1834
		ypArcsec    = 0.4021

		// Both routes are SOFA's, so agreement is exact up to the last few
		// bits — see coord/sofareference_test.go for the same bound argued in
		// full. Measured here at 9e-9 arcsec.
		agreementArcsec = 1e-6

		// One per cent of the constant, which covers the second-order term the
		// sine law drops and nothing else — rho is exact. Measured agreement
		// is within 0.2% at every site, and the bound is a hundred times
		// smaller than the effect it is checking for.
		magnitudeTolerance = 0.01
	)

	xp, yp := angle.Arcsec(xpArcsec).Radians(), angle.Arcsec(ypArcsec).Radians()

	time.RegisterModel(fixedEOP{dut1: dut1Seconds, xp: xp, yp: yp})

	atm := atmosphere.StandardRefraction
	atm.Model = atmosphere.RefractionNone{}

	utc1, utc2 := diurnalEpoch.UTC().JDParts()
	tt1, tt2 := diurnalEpoch.TT().JDParts()

	// The CIRS-to-ICRS rotation, so a CIRS direction can be handed to a
	// function that expects an ICRS one. Obtained here rather than from the
	// Context, which would make the comparison circular for the rotation.
	rc2i := gofaext.C2i06a(tt1, tt2)

	// The observer's rotation velocity points due east: it is perpendicular to
	// the meridian plane, so geodetic and geocentric east coincide and this is
	// the exact direction aberration displaces towards.
	east := coord.NewAltAz(angle.Zero(), angle.Deg(90))

	for _, s := range diurnalSites {
		site, err := coord.NewGeodetic(angle.Deg(s.lon), angle.Deg(s.lat), s.heightM)
		if err != nil {
			t.Fatalf("%s: NewGeodetic: %v", s.name, err)
		}

		ctx := coord.NewContext(diurnalEpoch, site, atm)
		diurabArcsec := expectedDiurnalArcsec(site.Lat().Radians(), site.Height())

		var worstAgainstSOFA, worstShift float64

		for _, d := range diurnalDirections {
			ri, di := angle.Hour(d.raHr).Radians(), angle.Deg(d.decDeg).Radians()

			aob, zob, _, _, _ := gofaext.Atio13(
				ri, di,
				utc1, utc2, dut1Seconds,
				site.Lon().Radians(), site.Lat().Radians(), site.Height(),
				xp, yp,
				0, atm.Temperature, atm.Humidity, atm.Wavelength,
			)

			sofa := coord.NewAltAz(angle.Rad(math.Pi/2-zob), angle.Rad(aob).Wrap360())

			// A billion au away, so subtracting the observer vector changes
			// the direction by nothing and the reduction is pure rotation plus
			// the aberration under test.
			far := icrsFromCIRS(rc2i, ri, di).MulScalar(1e9)

			got := ctx.GeocentricToObserved(far)

			if sep := altAzOffsetArcsec(got, sofa); sep > worstAgainstSOFA {
				worstAgainstSOFA = sep
			}

			if worstAgainstSOFA > agreementArcsec {
				t.Errorf("%s at RA %gh dec %g: GeocentricToObserved differs from SOFA's "+
					"Atio13 by %.4g arcsec, bound %g.\n"+
					"  Both are the same model; a difference here is the diurnal aberration "+
					"term being dropped, doubled, or applied along the wrong axis (#261).",
					s.name, d.raHr, d.decDeg, worstAgainstSOFA, agreementArcsec)
			}

			// The pre-fix answer: the same rotation with no aberration. The
			// difference between the two is the term this test exists for.
			before := rotateOnly(t, ctx, far)
			shift := altAzOffsetArcsec(got, before)

			if shift > worstShift {
				worstShift = shift
			}

			// Asserting the law per direction rather than only at its maximum
			// is what makes this a check on the physics: a maximum over a
			// sparse sweep never quite reaches the constant and would have to
			// be compared loosely, while sin(theta) is exact at every point.
			// coord.Separation, not altAzOffsetArcsec: theta runs to 180
			// degrees and that helper is a small-angle formula. It reads a
			// (longitude, latitude) pair through ICRS's constructor, and the
			// spherical geometry underneath is frame-agnostic.
			theta := coord.Separation(
				coord.NewICRS(before.Az(), before.Alt()),
				coord.NewICRS(east.Az(), east.Alt()),
			).Radians()

			want := diurabArcsec * math.Sin(theta)

			if diff := math.Abs(shift - want); diff > magnitudeTolerance*diurabArcsec {
				t.Errorf("%s at RA %gh dec %g: aberration shifted the place by %.4f arcsec, "+
					"and |v|/c·sin(theta) with theta=%.1f° says %.4f.\n"+
					"  The term is present but the wrong size or along the wrong axis, which "+
					"a comparison against another implementation of the same model would not "+
					"catch (#261).",
					s.name, d.raHr, d.decDeg, shift, theta*180/math.Pi, want)
			}
		}

		t.Logf("%-18s lat %+7.3f  |v|/c = %.4f arcsec, largest shift over the sweep %.4f, "+
			"vs SOFA %.2g", s.name, s.lat, diurabArcsec, worstShift, worstAgainstSOFA)
	}
}

// altAzOffsetArcsec is the angular offset between two horizon directions, in
// arcseconds, formed from the component differences rather than from an
// inverse cosine.
//
// The choice matters at both ends of this test. For two directions a
// microarcsecond apart, acos of the dot product is 1 − eps²/2, and float64
// stops resolving eps somewhere around a milliarcsecond — so the neighbouring
// altAzSeparationArcsec would report clean agreement for a defect a thousand
// times the bound asserted here. Projecting the azimuth difference by
// cos(elevation) is what makes it an angle on the sky rather than a coordinate
// difference, which near the zenith is most of the number.
func altAzOffsetArcsec(a, b coord.AltAz) float64 {
	dAz := a.Az().Degrees() - b.Az().Degrees()
	for dAz > 180 {
		dAz -= 360
	}

	for dAz <= -180 {
		dAz += 360
	}

	cross := dAz * math.Cos(b.Alt().Radians())
	along := a.Alt().Degrees() - b.Alt().Degrees()

	return math.Hypot(cross, along) * 3600
}

// icrsFromCIRS rotates a CIRS direction into ICRS: p_ICRS = rc2iᵀ · p_CIRS.
func icrsFromCIRS(rc2i [3][3]float64, ra, dec float64) vector.Vec3 {
	cosDec := math.Cos(dec)
	c := [3]float64{cosDec * math.Cos(ra), cosDec * math.Sin(ra), math.Sin(dec)}

	var out [3]float64

	for i := range 3 {
		for j := range 3 {
			out[i] += rc2i[j][i] * c[j]
		}
	}

	return vector.V3(out[0], out[1], out[2])
}

// rotateOnly reproduces what GeocentricToObserved did before #261: subtract
// the observer vector, rotate ICRS to ITRS, project into the local horizon.
// No aberration.
//
// Written out here rather than kept behind a flag in the production code,
// because a behaviour a caller can switch back on is a behaviour that has to
// be supported. Its only purpose is to be the thing the fix is measured
// against.
func rotateOnly(t *testing.T, ctx *coord.Context, v vector.Vec3) coord.AltAz {
	t.Helper()

	site := ctx.Site()
	eop := ctx.Time().EOP()

	tt1, tt2 := ctx.Time().TT().JDParts()
	utc1, utc2 := ctx.Time().UTC().JDParts()

	// The Context builds its matrix from the same routine. Rebuilding it here
	// rather than reading the cached one keeps the reconstruction independent
	// of the private state it is meant to shadow.
	mat := gofaext.C2t06a(tt1, tt2, utc1, utc2+eop.DUT1/86400.0, eop.XP, eop.YP)

	topo := v.Sub(ctx.ObsVec())

	tx := mat[0][0]*topo.X + mat[0][1]*topo.Y + mat[0][2]*topo.Z
	ty := mat[1][0]*topo.X + mat[1][1]*topo.Y + mat[1][2]*topo.Z
	tz := mat[2][0]*topo.X + mat[2][1]*topo.Y + mat[2][2]*topo.Z

	sinLat, cosLat := math.Sincos(site.Lat().Radians())
	sinLon, cosLon := math.Sincos(site.Lon().Radians())

	e := -sinLon*tx + cosLon*ty
	n := -sinLat*cosLon*tx - sinLat*sinLon*ty + cosLat*tz
	u := cosLat*cosLon*tx + cosLat*sinLon*ty + sinLat*tz

	return coord.NewAltAz(
		angle.Rad(math.Atan2(u, math.Hypot(e, n))),
		angle.Rad(math.Atan2(e, n)).Wrap360(),
	)
}

// expectedDiurnalArcsec is the diurnal aberration constant for a site at the
// given geodetic latitude and height, from first principles: the speed of the
// site's circular motion about the spin axis, over c.
//
// The rotation rate is the sidereal one — SOFA's iauPvtob writes it as
// 1.00273781191135448 · 2π / 86400 radians per UT1 second, the ratio of the
// sidereal to the solar day. Using 2π/86400 instead would be wrong by 0.27%,
// a quarter of the tolerance this feeds.
//
// rho comes from the WGS84 ellipsoid rather than from the Context's own
// observer vector, and the difference is not academic. That vector is ICRS, so
// its distance from the z axis is measured about the ICRS pole rather than the
// Celestial Intermediate Pole, and by 2026 those lie 0.14° apart. Near the
// equator the error is negligible; at 78° north, where rho is only 1330 km, it
// is 1.2% — over the tolerance below, and it was measured that way before this
// function was written the other way round.
func expectedDiurnalArcsec(latRad, heightM float64) float64 {
	const (
		earthRotationRadPerSec = 1.00273781191135448 * 2 * math.Pi / 86400.0

		// WGS84, the ellipsoid SOFA's Gd2gc uses for reference frame 1, which
		// is the one Apco13 and Pvtob pass.
		equatorialRadiusM = 6378137.0
		flattening        = 1.0 / 298.257223563
	)

	sinLat, cosLat := math.Sincos(latRad)
	e2 := 2*flattening - flattening*flattening

	// Radius of curvature in the prime vertical.
	n := equatorialRadiusM / math.Sqrt(1-e2*sinLat*sinLat)
	rhoMetres := (n + heightM) * cosLat

	return angle.Rad(earthRotationRadPerSec * rhoMetres /
		constants.SI2019.SpeedOfLight.Value).Arcseconds()
}
