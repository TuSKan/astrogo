package magnitude

import (
	"math"
	"sort"

	"github.com/TuSKan/astrogo/angle"
)

// ── Asteroid Phase-Curve Models ──────────────────────────────────────────────

// AsteroidHG computes the apparent V magnitude using the Bowell et al. (1989)
// H,G system. This is the legacy IAU model, still used by MPC with G=0.15
// for over 1.4 million objects.
//
//	V = H + 5·log₁₀(r·Δ) − 2.5·log₁₀((1−G)·Φ₁(α) + G·Φ₂(α))
//
// The basis functions follow Bowell et al. (1989) Eq. A4 exactly,
// matching the sbpy HG._hgphi implementation.
//
// Parameters:
//   - H: absolute magnitude
//   - G: slope parameter (default 0.15 from MPC)
//   - r: heliocentric distance (AU)
//   - delta: geocentric distance (AU)
//   - alpha: phase angle
func AsteroidHG(H, G, r, delta float64, alpha angle.Angle) float64 {
	a := alpha.Radians()
	phi1 := bowellPhi(a, 1)
	phi2 := bowellPhi(a, 2)

	phaseFn := (1-G)*phi1 + G*phi2
	if phaseFn <= 0 {
		phaseFn = 1e-30 // prevent log(0)
	}

	return H + 5*math.Log10(r*delta) - 2.5*math.Log10(phaseFn)
}

// bowellPhi evaluates the Bowell et al. (1989) basis function Φᵢ(α),
// Eq. A4. This is the exact form used in sbpy's HG._hgphi:
//
//	W   = exp(−90.56 · tan²(α/2))
//	Φˢ  = 1 − Cᵢ·sin(α)/(0.119 + 1.341·sin(α) − 0.754·sin²(α))
//	Φˡ  = exp(−Aᵢ · tan(α/2)^Bᵢ)
//	Φᵢ  = W·Φˢ + (1−W)·Φˡ
//
// where A = [3.332, 1.862], B = [0.631, 1.218], C = [0.986, 0.238].
func bowellPhi(alphaRad float64, i int) float64 {
	if alphaRad < 0 {
		alphaRad = 0
	}

	halfA := alphaRad / 2
	if halfA >= math.Pi/2 {
		return 0
	}

	var A, B, C float64

	switch i {
	case 1:
		A, B, C = 3.332, 0.631, 0.986
	case 2:
		A, B, C = 1.862, 1.218, 0.238
	default:
		return 0
	}

	sinA := math.Sin(alphaRad)
	tanHalf := math.Tan(halfA)

	// Opposition-effect weighting (narrow spike near α=0).
	W := math.Exp(-90.56 * tanHalf * tanHalf)

	// Small-angle (smooth) component.
	denom := 0.119 + 1.341*sinA - 0.754*sinA*sinA

	phiS := 1.0
	if math.Abs(denom) > 1e-30 {
		phiS = 1 - C*sinA/denom
	}

	// Large-angle (exponential decay) component.
	phiL := math.Exp(-A * math.Pow(tanHalf, B))

	return W*phiS + (1-W)*phiL
}

// ── HG1G2 — Muinonen et al. (2010) ──────────────────────────────────────────
// Current IAU standard (adopted 2012). Uses cubic spline basis functions
// from Muinonen et al. (2010) Table 3, ported from sbpy.

// AsteroidHG1G2 computes the apparent V magnitude using the Muinonen et al. (2010)
// H,G₁,G₂ three-parameter system.
//
//	V = H − 2.5·log₁₀(G₁·Φ₁(α) + G₂·Φ₂(α) + (1−G₁−G₂)·Φ₃(α)) + 5·log₁₀(r·Δ)
//
// The basis functions Φ₁, Φ₂, Φ₃ use cubic spline interpolation with
// knot values from Muinonen et al. (2010), exactly matching sbpy's
// HG1G2 implementation.
func AsteroidHG1G2(H, G1, G2, r, delta float64, alpha angle.Angle) float64 {
	a := alpha.Radians()
	G3 := 1 - G1 - G2

	phi1 := phi1Spline.eval(a)
	phi2 := phi2Spline.eval(a)
	phi3 := phi3Spline.eval(a)

	phaseFn := G1*phi1 + G2*phi2 + G3*phi3
	if phaseFn <= 0 {
		phaseFn = 1e-30
	}

	return H - 2.5*math.Log10(phaseFn) + 5*math.Log10(r*delta)
}

// ── HG12* — Penttilä et al. (2016) ──────────────────────────────────────────

// AsteroidHG12Star computes the apparent V magnitude using the Penttilä et al. (2016)
// revised H,G₁₂* single-parameter model with continuous derivatives.
//
// Uses the Penttilä (2016) mapping:
//
//	G₁ = 0.84293649 · G₁₂*
//	G₂ = 0.53513350 · (1 − G₁₂*)
func AsteroidHG12Star(H, G12star, r, delta float64, alpha angle.Angle) float64 {
	G1 := 0.84293649 * G12star
	G2 := 0.53513350 * (1 - G12star)

	return AsteroidHG1G2(H, G1, G2, r, delta, alpha)
}

// AsteroidHG12 computes the apparent V magnitude using the original
// Muinonen et al. (2010) H,G₁₂ model (discontinuous derivative at G₁₂=0.2).
//
// Uses the original Muinonen (2010) mapping:
//
//	G₁₂ < 0.2: G₁ = 0.7527·G₁₂ + 0.06164, G₂ = −0.9612·G₁₂ + 0.6270
//	G₁₂ ≥ 0.2: G₁ = 0.9529·G₁₂ + 0.02162, G₂ = −0.6125·G₁₂ + 0.5572
func AsteroidHG12(H, G12, r, delta float64, alpha angle.Angle) float64 {
	var G1, G2 float64
	if G12 < 0.2 {
		G1 = 0.7527*G12 + 0.06164
		G2 = -0.9612*G12 + 0.6270
	} else {
		G1 = 0.9529*G12 + 0.02162
		G2 = -0.6125*G12 + 0.5572
	}

	return AsteroidHG1G2(H, G1, G2, r, delta, alpha)
}

// ── sHG1G2 — Carry et al. (2024) ────────────────────────────────────────────
// "Combined spin orientation and phase function of asteroids"
// A&A, 687, A38 (2024). DOI: 10.1051/0004-6361/202449789
//
// Extends HG1G2 with a spin-geometry term to account for brightness changes
// due to polar oblateness and changing aspect angle over multiple apparitions.
// This is the model adopted by the FINK broker for LSST/Rubin Observatory.
//
// Reference implementation: phunk (https://github.com/maxmahlke/phunk)

// AsteroidSHG1G2 computes the apparent V magnitude using the Carry et al. (2024)
// sHG1G2 model — "spinned HG1G2" — which adds a spin correction to the standard
// HG1G2 phase function.
//
// The full equation (paper Eq. 5) is:
//
//	V = H + 5·log₁₀(r·Δ) − 2.5·log₁₀(G₁·Φ₁(α) + G₂·Φ₂(α) + (1−G₁−G₂)·Φ₃(α))
//	    + 2.5·log₁₀(1 − (1−R)·|cos Λ|)
//
// where Λ is the aspect angle between the observer's line of sight and the
// spin axis, and R is the polar-to-equatorial oblateness (0 < R ≤ 1).
//
// Parameters:
//   - H:        absolute magnitude
//   - G1, G2:   HG1G2 phase coefficients (G₃ = 1−G₁−G₂)
//   - r:        heliocentric distance (AU)
//   - delta:    geocentric distance (AU)
//   - alpha:    solar phase angle
//   - R:        polar-to-equatorial oblateness (Eq. 8: c(a+b)/(2ab), 0<R≤1; 1=sphere)
//   - cosLambda: cosine of the aspect angle (use CosAspectAngle to compute)
func AsteroidSHG1G2(H, G1, G2, r, delta float64, alpha angle.Angle, R, cosLambda float64) float64 {
	// Standard HG1G2 component (includes H + distance + phase terms).
	v := AsteroidHG1G2(H, G1, G2, r, delta, alpha)

	// Spin correction: s(α,δ) = 2.5·log₁₀(1 − (1−R)·|cos Λ|)  [Eq. 6]
	v += SpinCorrection(R, cosLambda)

	return v
}

// SpinCorrection computes the sHG1G2 spin-geometry term (Eq. 6 of Carry et al. 2024):
//
//	s = 2.5 · log₁₀(1 − (1−R) · |cos Λ|)
//
// This is always ≤ 0 (brightening when viewed pole-on). When R=1 (sphere),
// the correction is zero and sHG1G2 reduces to standard HG1G2.
//
// Parameters:
//   - R: polar-to-equatorial oblateness (0 < R ≤ 1)
//   - cosLambda: cosine of the aspect angle
func SpinCorrection(R, cosLambda float64) float64 {
	arg := 1 - (1-R)*math.Abs(cosLambda)
	if arg <= 0 {
		arg = 1e-30
	}

	return 2.5 * math.Log10(arg)
}

// CosAspectAngle computes the cosine of the aspect angle Λ between the
// observer's line of sight and the asteroid's spin axis (Eq. 7 of Carry et al. 2024):
//
//	cos Λ = sin(δ)·sin(δ₀) + cos(δ)·cos(δ₀)·cos(α−α₀)
//
// where (α,δ) are the equatorial coordinates (RA,Dec) of the asteroid at the time
// of observation, and (α₀,δ₀) are the equatorial coordinates of its spin axis.
// All inputs are angle.Angle values.
//
// Reference: IMCCE "Introduction to Ephemerides", Eq. 12.4.
func CosAspectAngle(ra, dec, ra0, dec0 angle.Angle) float64 {
	d := dec.Radians()
	d0 := dec0.Radians()
	dra := ra.Radians() - ra0.Radians()

	return math.Sin(d)*math.Sin(d0) + math.Cos(d)*math.Cos(d0)*math.Cos(dra)
}

// Oblateness computes the polar-to-equatorial oblateness R from tri-axial
// ellipsoid semi-axes a ≥ b ≥ c (Eq. 8 of Carry et al. 2024):
//
//	R = c·(a+b) / (2·a·b)
//
// Returns a value in (0, 1], where 1 means spherical.
func Oblateness(a, b, c float64) float64 {
	if a*b == 0 {
		return 1
	}

	return c * (a + b) / (2 * a * b)
}

// ── Cubic Spline (ported from sbpy) ──────────────────────────────────────────
// Natural cubic spline with specified endpoint derivatives.
// Outside the knot range, extrapolates linearly using endpoint derivatives.

type cubicSpline struct {
	x    []float64    // knot positions
	y    []float64    // knot values
	coef [][4]float64 // [a0, a1, a2, a3] per interval
	dyL  float64      // left endpoint derivative
	dyR  float64      // right endpoint derivative
}

func newCubicSpline(x, y []float64, dyL, dyR float64) *cubicSpline {
	n := len(y)
	if n < 2 {
		return &cubicSpline{x: x, y: y, dyL: dyL, dyR: dyR}
	}

	// Compute intervals and slopes.
	h := make([]float64, n-1)

	r := make([]float64, n-1)
	for i := range n - 1 {
		h[i] = x[i+1] - x[i]
		r[i] = (y[i+1] - y[i]) / h[i]
	}

	// Build tridiagonal system for internal derivatives.
	// B·d = C  where d are the derivatives at internal nodes.
	nInt := n - 2
	if nInt == 0 {
		// Only 2 knots — use endpoint derivatives directly.
		a0 := y[0]
		a1 := dyL
		a2 := (3*r[0] - 2*dyL - dyR) / h[0]
		a3 := (-2*r[0] + dyL + dyR) / (h[0] * h[0])

		return &cubicSpline{
			x:    x,
			y:    y,
			coef: [][4]float64{{a0, a1, a2, a3}},
			dyL:  dyL,
			dyR:  dyR,
		}
	}

	// Build and solve tridiagonal system.
	dys := make([]float64, n)
	dys[0] = dyL
	dys[n-1] = dyR

	// Right-hand side.
	C := make([]float64, nInt)
	for i := range nInt {
		k := i + 1
		C[i] = 3 * (r[k-1]*h[k] + r[k]*h[k-1])
	}

	C[0] -= dyL * h[1]
	C[nInt-1] -= dyR * h[nInt-1]

	// Tridiagonal matrix coefficients.
	lower := make([]float64, nInt)
	diag := make([]float64, nInt)

	upper := make([]float64, nInt)
	for i := range nInt {
		k := i + 1
		lower[i] = h[k]
		diag[i] = 2 * (h[k-1] + h[k])
		upper[i] = h[k-1]
	}

	// Thomas algorithm for tridiagonal solve.
	solveTridiagonal(lower, diag, upper, C)

	for i := range nInt {
		dys[i+1] = C[i]
	}

	// Build polynomial coefficients per interval.
	coef := make([][4]float64, n-1)
	for i := range n - 1 {
		a0 := y[i]
		a1 := dys[i]
		a2 := (3*r[i] - 2*dys[i] - dys[i+1]) / h[i]
		a3 := (-2*r[i] + dys[i] + dys[i+1]) / (h[i] * h[i])
		coef[i] = [4]float64{a0, a1, a2, a3}
	}

	return &cubicSpline{x: x, y: y, coef: coef, dyL: dyL, dyR: dyR}
}

// eval evaluates the spline at x, clipping negative values to zero
// (matching sbpy's _spline_positive).
func (s *cubicSpline) eval(xv float64) float64 {
	n := len(s.x)
	if n < 2 {
		if n == 1 {
			return math.Max(0, s.y[0])
		}

		return 0
	}

	var result float64

	switch {
	case xv < s.x[0]:
		// Linear extrapolation using left endpoint derivative.
		result = s.y[0] + s.dyL*(xv-s.x[0])
	case xv >= s.x[n-1]:
		// Linear extrapolation using right endpoint derivative.
		result = s.y[n-1] + s.dyR*(xv-s.x[n-1])
	default:
		// Find interval via binary search.
		idx := max(sort.SearchFloat64s(s.x, xv)-1, 0)

		if idx >= len(s.coef) {
			idx = len(s.coef) - 1
		}

		dx := xv - s.x[idx]
		c := s.coef[idx]
		result = c[0] + c[1]*dx + c[2]*dx*dx + c[3]*dx*dx*dx
	}

	// Clip negative to zero (sbpy _spline_positive).
	if result < 0 {
		return 0
	}

	return result
}

// solveTridiagonal solves a tridiagonal system using the Thomas algorithm.
// Modifies diag and rhs in place. Result is in rhs.
func solveTridiagonal(lower, diag, upper, rhs []float64) {
	n := len(rhs)
	if n == 0 {
		return
	}
	// Forward sweep.
	for i := 1; i < n; i++ {
		m := lower[i] / diag[i-1]
		diag[i] -= m * upper[i-1]
		rhs[i] -= m * rhs[i-1]
	}
	// Back substitution.
	rhs[n-1] /= diag[n-1]
	for i := n - 2; i >= 0; i-- {
		rhs[i] = (rhs[i] - upper[i]*rhs[i+1]) / diag[i]
	}
}

// ── Spline Knot Tables (Muinonen et al. 2010, Table 3) ──────────────────────
// Ported exactly from sbpy HG1G2._phi1v, _phi2v, _phi3v.

func deg2rad(degrees ...float64) []float64 {
	r := make([]float64, len(degrees))
	for i, d := range degrees {
		r[i] = d * math.Pi / 180
	}

	return r
}

var (
	// Φ₁ basis function spline.
	phi1Spline = newCubicSpline(
		deg2rad(7.5, 30.0, 60.0, 90.0, 120.0, 150.0),
		[]float64{7.5e-1, 3.3486016e-1, 1.3410560e-1, 5.1104756e-2, 2.1465687e-2, 3.6396989e-3},
		-1.9098593, -9.1328612e-2,
	)

	// Φ₂ basis function spline.
	phi2Spline = newCubicSpline(
		deg2rad(7.5, 30.0, 60.0, 90.0, 120.0, 150.0),
		[]float64{9.25e-1, 6.2884169e-1, 3.1755495e-1, 1.2716367e-1, 2.2373903e-2, 1.6505689e-4},
		-5.7295780e-1, -8.6573138e-8,
	)

	// Φ₃ basis function spline (opposition effect).
	phi3Spline = newCubicSpline(
		deg2rad(0.0, 0.3, 1.0, 2.0, 4.0, 8.0, 12.0, 20.0, 30.0),
		[]float64{1.0, 8.3381185e-1, 5.7735424e-1, 4.2144772e-1, 2.3174230e-1, 1.0348178e-1, 6.1733473e-2, 1.6107006e-2, 0.0},
		-1.0630097, 0,
	)
)
