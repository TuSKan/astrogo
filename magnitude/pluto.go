package magnitude

import (
	"fmt"
	"math"
	"sync"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// PlutoCharonMagnitudes are Johnson V apparent magnitudes of Pluto, of Charon,
// and of the two unresolved, which is what a telescope that cannot split them
// measures. Combined adds the two fluxes, not the magnitudes.
type PlutoCharonMagnitudes struct {
	Pluto    float64
	Charon   float64
	Combined float64
}

// PlutoCharonApparent returns the V magnitudes of Pluto and Charon at t from
// the light curves Buie et al. (2010, AJ 139, 1117) fitted to Hubble ACS/HRC
// photometry that resolved the two: each body's rotational light curve as a
// Fourier series in its sub-Earth east longitude (Tables 8 and 12), and its
// phase curve as a Hapke model of a uniform sphere (Table 9), which for
// Charon, with its sharp opposition surge, no linear law can stand in for.
// The two are computed separately and combined in flux.
//
// This is the alternative to PlanetApparent(p, eph.Pluto, t), which uses the
// historical law V = −1.01 + 5 log10(rΔ) + 0.041α and agrees with JPL
// Horizons. The two do not agree with each other, and the difference is the
// reason both exist:
//
//   - At its own epoch this model is measurement. It reproduces the paper's
//     twelve Hubble visits of 2002–2003 to a mean |O−C| of 0.004 mag for
//     Pluto and 0.005 for Charon, 0.014 at worst, the paper's own fit
//     residual; at the first of them the historical law is 0.27 mag too bright.
//   - It is a 2002–2003 calibration, made with Earth 28–33° from Pluto's
//     equator. In the 2020s that angle is 55–59°, and this model comes out
//     0.16–0.36 mag fainter than the historical law and Horizons. Which is
//     nearer Pluto's brightness now is not established: Pluto's light curve
//     shrinks and its mean brightness changes as it turns pole-on (the paper
//     measures both between 1993 and 2003), and this model has no latitude
//     term. Contemporary V photometry is what would decide it.
//
// It does not model mutual events, which dim the pair by up to 0.5 mag during
// an eclipse season, or anything outside the phase angles Earth sees
// (at most about 1.9°).
func PlutoCharonApparent(p eph.Provider, t time.Time) (PlutoCharonMagnitudes, error) {
	r, delta, phAng, lon, err := plutoCharonGeometry(p, t)
	if err != nil {
		return PlutoCharonMagnitudes{}, err
	}

	return plutoCharonMagnitudes(r, delta, phAng, lon), nil
}

// plutoCharonGeometry returns what the model is evaluated at: Pluto's
// heliocentric and geocentric distances (AU), its phase angle (degrees), and
// its sub-Earth east longitude in the paper's system (degrees).
func plutoCharonGeometry(p eph.Provider, t time.Time) (r, delta, phAng, lon float64, err error) {
	plutoSt, err := p.State(eph.Pluto, t)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("magnitude: Pluto state: %w", err)
	}

	sunSt, err := p.State(eph.Sun, t)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("magnitude: sun state: %w", err)
	}

	sunToPluto := [3]float64{plutoSt.Pos.X - sunSt.Pos.X, plutoSt.Pos.Y - sunSt.Pos.Y, plutoSt.Pos.Z - sunSt.Pos.Z}
	observerToPluto := [3]float64{plutoSt.Pos.X, plutoSt.Pos.Y, plutoSt.Pos.Z}

	r = vecLen(sunToPluto)
	delta = vecLen(observerToPluto)
	phAng = angleBetween(sunToPluto, observerToPluto) * 180 / math.Pi

	// Pluto is seen as it was when the light left it: at 30–50 AU that is
	// 4–7 hours, 10–16° of its rotation.
	jd1, jd2 := t.TDB().JDParts()
	d := (jd1 - 2451545.0) + jd2 - delta/lightAUPerDay

	return r, delta, phAng, plutoSubObserverLongitude(negate(observerToPluto), d), nil
}

// plutoCharonMagnitudes evaluates the model at distances r and delta (AU),
// phase angle phAng (degrees) and sub-Earth east longitude lon on Pluto in the
// paper's system (degrees). Charon faces Pluto, so its sub-Earth longitude in
// the paper's system is Pluto's plus 180°.
func plutoCharonMagnitudes(r, delta, phAng, lon float64) PlutoCharonMagnitudes {
	// The paper reduces its photometry to mean opposition distance, r = 39.5
	// AU and Δ = 38.5 AU, "in keeping with past publications".
	dist := 5 * math.Log10(r*delta/(39.5*38.5))

	pluto := plutoLightCurve.at(lon) + plutoPhaseCurve.at(phAng) + dist
	charon := charonLightCurve.at(lon+180) + charonPhaseCurve.at(phAng) + dist

	return PlutoCharonMagnitudes{Pluto: pluto, Charon: charon, Combined: addMagnitudes(pluto, charon)}
}

// addMagnitudes is the magnitude of two sources' fluxes together.
func addMagnitudes(a, b float64) float64 {
	return -2.5 * math.Log10(math.Pow(10, -0.4*a)+math.Pow(10, -0.4*b))
}

// lightCurve is a rotational light curve as Buie et al. tabulate it: the mean
// magnitude at zero phase and mean opposition distance, and Fourier
// coefficients in sub-Earth east longitude,
//
//	m(λ) = a0 + Σ [aₙ cos nλ + bₙ sin nλ].
type lightCurve struct {
	a0   float64
	a, b []float64
}

func (c lightCurve) at(lonDeg float64) float64 {
	m := c.a0
	x := lonDeg * math.Pi / 180

	for n := range c.a {
		k := float64(n + 1)
		m += c.a[n]*math.Cos(k*x) + c.b[n]*math.Sin(k*x)
	}

	return m
}

// plutoLightCurve is Buie et al. (2010) Table 8, V, four terms.
var plutoLightCurve = lightCurve{
	a0: 15.3298,
	a:  []float64{+0.0338, -0.0373, +0.0038, +0.0035},
	b:  []float64{+0.0969, -0.0247, +0.0046, +0.0033},
}

// charonLightCurve is Buie et al. (2010) Table 12, V, two terms.
var charonLightCurve = lightCurve{
	a0: 17.0978,
	a:  []float64{-0.0440, +0.0129},
	b:  []float64{-0.0009, +0.0078},
}

// plutoSubObserverLongitude is the east longitude, in degrees and in Buie et
// al.'s system, of the point on Pluto under the direction dir from its
// center, d days (TDB) after J2000.0.
//
// Their system (footnote 4, after Buie et al. 1997) is right-handed, with the
// north pole along the rotational angular momentum and 0° through the
// sub-Charon point at periapse, from their Charon orbit. The IAU model (NAIF
// pck00010 and pck00011 agree) has the same pole sense but its own orbit and
// prime meridian. Against the twelve sub-Earth longitudes the paper tabulates
// (Table 3), the IAU model, light time included, runs 1.96° ahead, with a
// spread of 0.06° that is the tabulation's own; the same with DE440 and with
// the analytical ephemeris. 1.96° is subtracted. Its latitude differs by 1.0°,
// which the model does not use.
func plutoSubObserverLongitude(dir [3]float64, d float64) float64 {
	const (
		poleRA, poleDec  = 132.993, -6.163
		w0, wRate        = 302.695, 56.3625225
		paperSystemShift = 1.96
	)

	east := subPointEastLongitude(dir, poleRA, poleDec, w0+wRate*d)

	return angle.Deg(east - paperSystemShift).Wrap360().Degrees()
}

// subPointEastLongitude is the east longitude, in degrees, of the point on a
// body under the direction dir from its center, for a pole at right ascension
// raDeg and declination decDeg and a prime meridian at wDeg, IAU style: W is
// measured east along the body's equator from the node on the ICRF equator,
// so a direction at angle θ east of the node lies at east longitude θ − W.
func subPointEastLongitude(dir [3]float64, raDeg, decDeg, wDeg float64) float64 {
	const deg = math.Pi / 180

	sinRA, cosRA := math.Sincos(raDeg * deg)
	sinDec, cosDec := math.Sincos(decDeg * deg)

	// The node, and the direction 90° east of it along the body's equator:
	// the pole crossed with the node.
	node := [3]float64{-sinRA, cosRA, 0}
	east := [3]float64{-sinDec * cosRA, -sinDec * sinRA, cosDec}

	theta := math.Atan2(dot(dir, east), dot(dir, node)) / deg

	return angle.Deg(theta - wDeg).Wrap360().Degrees()
}

// hapkeSphere is a uniform sphere under Hapke's (1993) bidirectional
// reflectance with the shadow-hiding opposition effect and macroscopic
// roughness, the model Buie et al. (2010) fit to each body's phase curve
// (Table 9). P is the single-particle phase function averaged over the phase
// angles observed, as the paper tabulates it: a constant.
type hapkeSphere struct {
	w, h, b0, p, theta float64 // theta in degrees

	zeroOnce sync.Once
	zero     float64
}

// plutoPhaseCurve and charonPhaseCurve are Buie et al. (2010) Table 9, V.
var (
	plutoPhaseCurve  = &hapkeSphere{w: 0.7303, h: 0.0790, b0: 0.790, p: 2.83, theta: 10}
	charonPhaseCurve = &hapkeSphere{w: 0.6737, h: 0.0044, b0: 0.600, p: 2.46, theta: 20}
)

// at is the phase correction at phAng degrees: the sphere's magnitude there
// less its magnitude at zero phase.
//
// Evaluated from the paper's parameters it is +0.0438 for Pluto and +0.2636
// for Charon at 1°, where the paper's figure captions give +0.0398 and
// +0.2549. The 0.004 and 0.009 mag differences are not explained by anything
// the paper states (its roughness implementation is the likeliest place:
// roughness alone moves these by 0.002 and 0.007), and they are under its own
// fit residual of 0.013; against the photometry itself this reproduces every
// visit to 0.014 or better.
func (s *hapkeSphere) at(phAng float64) float64 {
	s.zeroOnce.Do(func() { s.zero = s.flux(0) })

	return -2.5 * math.Log10(s.flux(phAng*math.Pi/180)/s.zero)
}

// hapkeGrid is the number of radial steps in the disk integration, and half
// the number of azimuthal ones. At 64 the phase curve has converged to 2e-5
// mag against 200.
const hapkeGrid = 64

// flux integrates the reflectance over the lit, visible disk of a unit sphere
// seen from +z with the Sun at phase angle g (radians) in the x–z plane. The
// projected area element is dx dy = μ dA, so the integrand is r itself. The
// radius runs as sin t, which takes the √(1−ρ²) out of the limb.
func (s *hapkeSphere) flux(g float64) float64 {
	sunX, sunZ := math.Sin(g), math.Cos(g)
	dt := math.Pi / 2 / hapkeGrid
	dphi := math.Pi / hapkeGrid

	var sum float64

	for a := range hapkeGrid {
		t := (float64(a) + 0.5) * dt
		rho := math.Sin(t)
		mu := math.Cos(t)

		for b := range 2 * hapkeGrid {
			phi := (float64(b) + 0.5) * dphi
			mu0 := rho*math.Cos(phi)*sunX + mu*sunZ
			sum += s.reflectance(mu0, mu, g) * rho * math.Cos(t) * dt * dphi
		}
	}

	return sum
}

// reflectance is Hapke's bidirectional reflectance for incidence and emission
// cosines mu0 and mu at phase g (radians): the isotropic multiple-scattering
// approximation with the shadow-hiding opposition effect,
//
//	r = w/4π · μ0e/(μ0e+μe) · [(1+B(g)) P + H(μ0e) H(μe) − 1] · S,
//	B(g) = B0 / (1 + tan(g/2)/h),
//
// with the effective cosines μ0e, μe and shadowing function S of the
// macroscopic roughness correction (Hapke 1984), and H(x) = (1+2x)/(1+2γx),
// γ = √(1−w). The constant factor w/4π cancels in the phase curve and is kept
// for the formula's sake.
func (s *hapkeSphere) reflectance(mu0, mu, g float64) float64 {
	if mu0 <= 0 || mu <= 0 {
		return 0
	}

	mu0e, mue, shadow := s.roughness(mu0, mu, g)

	gamma := math.Sqrt(1 - s.w)
	hFunc := func(x float64) float64 { return (1 + 2*x) / (1 + 2*gamma*x) }

	b := s.b0 / (1 + math.Tan(g/2)/s.h)

	return s.w / (4 * math.Pi) * mu0e / (mu0e + mue) * ((1+b)*s.p + hFunc(mu0e)*hFunc(mue) - 1) * shadow
}

// roughness returns the effective incidence and emission cosines and the
// shadowing function of Hapke's (1984) macroscopic roughness correction for a
// mean slope of s.theta degrees.
func (s *hapkeSphere) roughness(mu0, mu, g float64) (mu0e, mue, shadow float64) {
	i := math.Acos(math.Min(1, mu0))
	e := math.Acos(math.Min(1, mu))

	// psi, the azimuth between the planes of incidence and emission.
	var psi float64

	if si, se := math.Sin(i), math.Sin(e); si*se > 1e-12 {
		psi = math.Acos(math.Max(-1, math.Min(1, (math.Cos(g)-mu0*mu)/(si*se))))
	}

	tanT := math.Tan(s.theta * math.Pi / 180)
	cotT := 1 / tanT
	chi := 1 / math.Sqrt(1+math.Pi*tanT*tanT)

	// E1 and E2 vanish at normal incidence or emission, where cot y diverges.
	e1 := func(y float64) float64 {
		if y < 1e-9 {
			return 0
		}

		return math.Exp(-2 / math.Pi * cotT / math.Tan(y))
	}
	e2 := func(y float64) float64 {
		if y < 1e-9 {
			return 0
		}

		c := cotT / math.Tan(y)

		return math.Exp(-c * c / math.Pi)
	}
	eta := func(y float64) float64 {
		return chi * (math.Cos(y) + math.Sin(y)*tanT*e2(y)/(2-e1(y)))
	}

	f := math.Exp(-2 * math.Tan(psi/2))
	half := math.Sin(psi / 2)
	half *= half

	if i <= e {
		den := 2 - e1(e) - psi/math.Pi*e1(i)
		mu0e = chi * (math.Cos(i) + math.Sin(i)*tanT*(math.Cos(psi)*e2(e)+half*e2(i))/den)
		mue = chi * (math.Cos(e) + math.Sin(e)*tanT*(e2(e)-half*e2(i))/den)
		shadow = mue / eta(e) * mu0 / eta(i) * chi / (1 - f + f*chi*mu0/eta(i))

		return mu0e, mue, shadow
	}

	den := 2 - e1(i) - psi/math.Pi*e1(e)
	mu0e = chi * (math.Cos(i) + math.Sin(i)*tanT*(e2(i)-half*e2(e))/den)
	mue = chi * (math.Cos(e) + math.Sin(e)*tanT*(math.Cos(psi)*e2(i)+half*e2(e))/den)
	shadow = mue / eta(e) * mu0 / eta(i) * chi / (1 - f + f*chi*mu/eta(e))

	return mu0e, mue, shadow
}
