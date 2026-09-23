package magnitude

import (
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/time"
)

// ── Mars — Mallama & Hilton 2018, Eq. 6–7 ───────────────────────────────────
// Two phase-curve regimes at the α = 50° boundary, plus the model's two Mars
// corrections: one for the face Mars turns toward the Sun and the observer, one
// for its season.
//
// Skyfield leaves both corrections out — its _mars_magnitude sets them to zero
// "until effects from Mars rotation are written up" — and astrogo did too until
// #389, when it was up to 0.076 mag from JPL Horizons, which applies them.

// marsMag is the apparent magnitude at distances r and delta (AU) and phase
// angle phAng, for an effective central meridian centralMeridian and a solar
// longitude ls (all degrees; see marsAngles).
func marsMag(r, delta, phAng, centralMeridian, ls float64) float64 {
	rMag := 2.5 * math.Log10(r*r)
	deltaMag := 2.5 * math.Log10(delta*delta)
	distMod := rMag + deltaMag

	const geocentricLimit = 50.0

	var (
		phAngFactor float64
		v10         float64
	)
	if phAng <= geocentricLimit {
		v10 = -1.601
		phAngFactor = 2.267e-02*phAng - 1.302e-04*phAng*phAng
	} else {
		v10 = -0.367
		phAngFactor = -0.02573*phAng + 0.0003445*phAng*phAng
	}

	return v10 + distMod + phAngFactor +
		marsCorrection(&marsRotationCorrection, centralMeridian) +
		marsCorrection(&marsOrbitalCorrection, ls)
}

// marsRotationCorrection is the magnitude correction for the effective central
// meridian, Mallama (2007, Icarus 192, 404) Table 6, as the model's published
// code carries it: 10° steps, entry k at 10·(k−2)°, so the table runs from −20°
// to 370° and its first and last two entries repeat the other end. Five-point
// interpolation then never has to wrap.
var marsRotationCorrection = [40]float64{
	0.024, 0.034, 0.036, 0.045, 0.038, 0.023, 0.015, 0.011,
	0.000, -0.012, -0.018, -0.036, -0.044, -0.059, -0.060, -0.055,
	-0.043, -0.041, -0.041, -0.036, -0.036, -0.018, -0.038, -0.011,
	0.002, 0.004, 0.018, 0.019, 0.035, 0.050, 0.035, 0.027,
	0.037, 0.048, 0.025, 0.022, 0.024, 0.034, 0.036, 0.045,
}

// marsOrbitalCorrection is the magnitude correction for the solar longitude Ls,
// Mallama (2007) Table 8, laid out as marsRotationCorrection is.
var marsOrbitalCorrection = [40]float64{
	-0.030, -0.017, -0.029, -0.017, -0.014, -0.006, -0.018, -0.020,
	-0.014, -0.030, -0.008, -0.040, -0.024, -0.037, -0.036, -0.032,
	0.010, 0.010, -0.001, 0.044, 0.025, -0.004, -0.016, -0.008,
	0.029, -0.054, -0.033, 0.055, 0.017, 0.052, 0.006, 0.087,
	0.006, 0.064, 0.030, 0.019, -0.030, -0.017, -0.029, -0.017,
}

// marsCorrection interpolates a correction table at deg degrees, as the
// model's published code does.
//
// That is five-point Stirling interpolation with one difference missing, and
// the missing one is deliberate here. The published Fortran (Ap_Mag_V3.f90,
// Mars_Stirling) forms three first differences where the formula needs four —
// its loop runs to 2, beside a comment citing Python's range(4) — so the fourth
// is zero and the third and fourth differences built on it are not Stirling's.
// Horizons computes it the same way. From Horizons' own geometry, this form
// reproduces its APmag on 11 dates from 2003 to 2026 to 0.0006 mag, which is
// Horizons' rounding; textbook Stirling misses by up to 0.0029. So the model as
// its authors' code and Horizons both publish it is this one, and correcting
// the interpolation would disagree with both. The difference is a few
// thousandths of a magnitude, a small fraction of the model's own scatter.
//
// NaN for a non-finite deg.
func marsCorrection(table *[40]float64, deg float64) float64 {
	if math.IsNaN(deg) || math.IsInf(deg, 0) {
		return math.NaN()
	}

	// Only an angle outside the circle goes through the wrap: its trip through
	// radians would move one inside it off the table's nodes.
	if deg < 0 || deg >= 360 {
		deg = angle.Deg(deg).Wrap360().Degrees()
	}

	// That trip can also round an angle just short of 360° up to 360° itself.
	// Clamping keeps it in the table, where p = 1 from the last interval lands
	// exactly on the 360° node.
	zero := min(int(deg/10), 35)
	p := deg/10 - float64(zero)
	f := table[zero : zero+5]

	d1 := [4]float64{f[1] - f[0], f[2] - f[1], f[3] - f[2], 0}
	d2 := [3]float64{d1[1] - d1[0], d1[2] - d1[1], d1[3] - d1[2]}
	d3 := [2]float64{d2[1] - d2[0], d2[2] - d2[1]}
	d4 := d3[1] - d3[0]

	a4 := d4 / 24
	a3 := (d3[0] + d3[1]) / 12
	a2 := d2[1]/2 - a4
	a1 := (d1[1]+d1[2])/2 - a3

	return f[2] + p*(a1+p*(a2+p*(a3+p*a4)))
}

// marsAngles returns the two angles the Mars corrections are tabulated in, in
// degrees, from the heliocentric and observer-centric vectors to Mars (ICRF,
// AU) at t.
//
// The effective central meridian is the longitude midway between the face
// Mars turns to the observer and the face it turns to the Sun. Ls is Mars's
// heliocentric J2000 ecliptic longitude less 85°, the model's approximation to
// the areocentric solar longitude.
func marsAngles(sunToPlanet, observerToPlanet [3]float64, delta float64, t time.Time) (centralMeridian, ls float64) {
	subObserver, subSolar := marsSubLongitudes(sunToPlanet, observerToPlanet, delta, t)

	return marsEffectiveCentralMeridian(subObserver, subSolar), marsSolarLongitude(eclipticLongitude(sunToPlanet))
}

// marsSubLongitudes returns the west longitudes, in degrees, of the points on
// Mars under the observer and under the Sun.
//
// Mars is seen as it was when the light left it, so its rotation is evaluated
// a light time before t. That matters: at 2.4 AU the light time is 20 minutes,
// which Mars turns through 4.9° of longitude.
func marsSubLongitudes(sunToPlanet, observerToPlanet [3]float64, delta float64, t time.Time) (subObserver, subSolar float64) {
	jd1, jd2 := t.TDB().JDParts()
	d := (jd1 - 2451545.0) + jd2 - delta/lightAUPerDay

	return marsWestLongitude(negate(observerToPlanet), d), marsWestLongitude(negate(sunToPlanet), d)
}

// marsEffectiveCentralMeridian is the mean of the sub-observer and sub-solar
// west longitudes (degrees), taken across whichever arc between them is
// shorter.
func marsEffectiveCentralMeridian(subObserver, subSolar float64) float64 {
	cm := (subObserver + subSolar) / 2
	if math.Abs(subObserver-subSolar) > 180 {
		cm += 180
	}

	return angle.Deg(cm).Wrap360().Degrees()
}

// marsSolarLongitude is the model's Ls for a heliocentric J2000 ecliptic
// longitude (degrees).
func marsSolarLongitude(eclipticLon float64) float64 {
	return angle.Deg(eclipticLon - 85).Wrap360().Degrees()
}

// eclipticLongitude is the J2000 ecliptic longitude of an ICRF vector, in
// degrees: its longitude after a rotation about x by the obliquity.
func eclipticLongitude(v [3]float64) float64 {
	sinEps, cosEps := math.Sincos(constants.IAU.ObliquityJ2000.Value)

	return math.Atan2(v[1]*cosEps+v[2]*sinEps, v[0]) * 180 / math.Pi
}

// marsWestLongitude is the west longitude, in degrees, of the point on Mars
// under the direction dir from its center, d days (TDB) after J2000.0.
//
// Mars's orientation is IAU 2009 (Archinal et al. 2011), as NAIF's pck00010
// carries it. The pole and prime meridian of IAU 2015 differ from it by far
// less than the tables resolve: the corrections change by at most 0.009 mag
// per degree of longitude.
func marsWestLongitude(dir [3]float64, d float64) float64 {
	const deg = math.Pi / 180

	cent := d / 36525
	ra := (317.68143 - 0.1061*cent) * deg
	dec := (52.88650 - 0.0609*cent) * deg
	w := 176.630 + 350.89198226*d

	// The node of Mars's equator on the ICRF equator, and the direction 90°
	// east of it along Mars's equator: the pole crossed with the node.
	sinRA, cosRA := math.Sincos(ra)
	sinDec, cosDec := math.Sincos(dec)
	node := [3]float64{-sinRA, cosRA, 0}
	east := [3]float64{-sinDec * cosRA, -sinDec * sinRA, cosDec}

	// W is measured east from the node to the prime meridian, so a direction at
	// angle theta east of the node lies at east longitude theta − W.
	theta := math.Atan2(dot(dir, east), dot(dir, node)) / deg

	return angle.Deg(w - theta).Wrap360().Degrees()
}

// lightAUPerDay is the speed of light in astronomical units per day.
var lightAUPerDay = constants.SI2019.SpeedOfLight.Value *
	constants.Derived.JulianDaySeconds.Value / constants.IAU.AstronomicalUnit.Value

func negate(v [3]float64) [3]float64 { return [3]float64{-v[0], -v[1], -v[2]} }

func dot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
