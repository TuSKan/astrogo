package fits

import (
	"fmt"
	"math"
	"strings"
)

// WCS defines the mathematical boundaries transforming image pixel matrices
// into absolute scientific world coordinates (e.g., Right Ascension / Declination mapping).
type WCS struct {
	sipA    map[[2]int]float64
	tpv2    map[int]float64
	tpv1    map[int]float64
	sipBP   map[[2]int]float64
	sipAP   map[[2]int]float64
	sipB    map[[2]int]float64
	cdelt   []float64
	pc      [][]float64
	ctype   []string
	crval   []float64
	crpix   []float64
	latAxis int
	lonAxis int
	nAxis   int
}

// NewWCS constructs an empty identity-mapped N-dimensional Coordinate System.
func NewWCS(naxis int) *WCS {
	pc := make([][]float64, naxis)
	for i := range naxis {
		pc[i] = make([]float64, naxis)
		pc[i][i] = 1.0 // Initialize with standard Identity matrix
	}

	return &WCS{
		nAxis: naxis,
		crpix: make([]float64, naxis),
		crval: make([]float64, naxis),
		cdelt: make([]float64, naxis),
		ctype: make([]string, naxis),
		pc:    pc,
	}
}

// SetCRPIX sets the reference pixel coordinate array (1-based FITS indexing).
func (w *WCS) SetCRPIX(crpix []float64) {
	w.crpix = crpix
}

// SetCRVAL sets the world coordinate values at the reference pixel.
func (w *WCS) SetCRVAL(crval []float64) {
	w.crval = crval
}

// SetCDELT sets the pixel scale (degrees per pixel) along each axis.
func (w *WCS) SetCDELT(cdelt []float64) {
	w.cdelt = cdelt
}

// SetCTYPE sets the coordinate axis type identifiers (e.g., "RA---TAN", "DEC--TAN").
func (w *WCS) SetCTYPE(ctype []string) {
	w.ctype = ctype
}

// SetPC sets the linear transformation (rotation/skew) matrix.
func (w *WCS) SetPC(pc [][]float64) {
	w.pc = pc
}

// SetSIP sets the forward SIP distortion polynomial coefficients.
// Each map key [2]int{p, q} represents the exponent pair for u^p * v^q.
// The maps may be nil to disable SIP distortion.
func (w *WCS) SetSIP(a, b map[[2]int]float64) {
	w.sipA = a
	w.sipB = b
}

// SetSIPInverse sets the inverse SIP polynomial coefficients (AP, BP).
// These are used in WorldToPixel to compute a direct inverse without iteration.
func (w *WCS) SetSIPInverse(ap, bp map[[2]int]float64) {
	w.sipAP = ap
	w.sipBP = bp
}

// SetTPV sets the TPV distortion polynomial coefficients.
// pv1 and pv2 map term index (0–39) to coefficient value.
// pv1 corrects the longitude intermediate coordinate;
// pv2 corrects the latitude intermediate coordinate.
func (w *WCS) SetTPV(pv1, pv2 map[int]float64) {
	w.tpv1 = pv1
	w.tpv2 = pv2
}

// GetCRPIX returns the reference pixel coordinate array.
func (w *WCS) GetCRPIX() []float64 {
	return w.crpix
}

// GetCRVAL returns the world coordinate values at the reference pixel.
func (w *WCS) GetCRVAL() []float64 {
	return w.crval
}

// GetCDELT returns the pixel scale along each axis.
func (w *WCS) GetCDELT() []float64 {
	return w.cdelt
}

// GetCTYPE returns the coordinate axis type identifiers.
func (w *WCS) GetCTYPE() []string {
	return w.ctype
}

// GetPC returns the linear transformation matrix.
func (w *WCS) GetPC() [][]float64 {
	return w.pc
}

const (
	deg2rad = math.Pi / 180.0
	rad2deg = 180.0 / math.Pi
)

// sipEval evaluates a SIP polynomial: Σ coeffs[(p,q)] · u^p · v^q.
//
// Uses precomputed power tables instead of math.Pow for performance:
// SIP is evaluated per-pixel, so on a 4096×4096 image this runs ~16M times.
func sipEval(coeffs map[[2]int]float64, u, v float64) float64 {
	if len(coeffs) == 0 {
		return 0
	}

	// Determine maximum exponents to size the power tables.
	var maxP, maxQ int
	for pq := range coeffs {
		if pq[0] > maxP {
			maxP = pq[0]
		}

		if pq[1] > maxQ {
			maxQ = pq[1]
		}
	}

	// Precompute u^0..u^maxP and v^0..v^maxQ.
	upow := make([]float64, maxP+1)
	vpow := make([]float64, maxQ+1)
	upow[0] = 1
	vpow[0] = 1

	for i := 1; i <= maxP; i++ {
		upow[i] = upow[i-1] * u
	}

	for i := 1; i <= maxQ; i++ {
		vpow[i] = vpow[i-1] * v
	}

	var sum float64
	for pq, c := range coeffs {
		sum += c * upow[pq[0]] * vpow[pq[1]]
	}

	return sum
}

// PixelToWorld transforms a continuous FITS 1-indexed pixel coordinate
// slice mapping it against spherical Sky metrics.
//
// Supported projections: TAN (Gnomonic), SIN (Orthographic),
// ARC (Zenithal equidistant), STG (Stereographic), AIT (Hammer-Aitoff).
// SIP distortion is applied when CTYPE contains the "-SIP" suffix.
func (w *WCS) PixelToWorld(pixels []float64) ([]float64, error) {
	if len(pixels) != w.nAxis {
		return nil, fmt.Errorf("%w: expected %d", ErrWCSDimension, w.nAxis)
	}

	// Detect projection type and set lonAxis/latAxis (needed by SIP and deprojection).
	proj := w.extractProjection()

	// Step 1: Compute pixel offsets from reference pixel.
	offset := make([]float64, w.nAxis)
	for i := range w.nAxis {
		offset[i] = pixels[i] - w.crpix[i]
	}

	// Step 1b: Apply SIP forward distortion to the celestial pixel offsets.
	// SIP corrects (u,v) → (u + f(u,v), v + g(u,v)) before the CD matrix.
	if w.nAxis >= 2 && len(w.sipA) > 0 && w.lonAxis >= 0 {
		u := offset[w.lonAxis]
		v := offset[w.latAxis]
		offset[w.lonAxis] = u + sipEval(w.sipA, u, v)
		offset[w.latAxis] = v + sipEval(w.sipB, u, v)
	}

	// Step 2: Linear Transformation mapping offsets onto standard linear plane.
	inter := make([]float64, w.nAxis)
	for i := range w.nAxis {
		var sum float64
		for j := range w.nAxis {
			sum += w.pc[i][j] * offset[j]
		}

		inter[i] = sum * w.cdelt[i]
	}

	// Step 2b: Apply TPV distortion in intermediate coordinate space.
	// TPV replaces (x, y) with polynomial functions of (x, y).
	if w.nAxis >= 2 && len(w.tpv1) > 0 && w.lonAxis >= 0 {
		x := inter[w.lonAxis]
		y := inter[w.latAxis]
		inter[w.lonAxis] = tpvEval(w.tpv1, x, y)
		inter[w.latAxis] = tpvEval(w.tpv2, x, y)
	}

	// Step 3: Spherical de-projection if celestial axes are present.
	if w.nAxis >= 2 && proj != "" {
		// Map intermediate coords using axis order detected from CTYPE.
		xRad := inter[w.lonAxis] * deg2rad
		yRad := inter[w.latAxis] * deg2rad

		alpha0 := w.crval[w.lonAxis] * deg2rad
		delta0 := w.crval[w.latAxis] * deg2rad

		ra, dec, err := deproject(proj, xRad, yRad, alpha0, delta0)
		if err != nil {
			return nil, err
		}

		// Normalize RA into [0, 2π).
		if ra < 0 {
			ra += 2 * math.Pi
		}

		ra = math.Mod(ra, 2*math.Pi)

		res := make([]float64, w.nAxis)
		res[w.lonAxis] = ra * rad2deg

		res[w.latAxis] = dec * rad2deg
		for i := range w.nAxis {
			if i != w.lonAxis && i != w.latAxis {
				res[i] = w.crval[i] + inter[i]
			}
		}

		return res, nil
	}

	// Fallback: linear mapping.
	res := make([]float64, w.nAxis)
	for i := range w.nAxis {
		res[i] = w.crval[i] + inter[i]
	}

	return res, nil
}

// WorldToPixel converts world coordinates back to FITS 1-indexed pixel coordinates.
//
// For recognized spherical projections (TAN, SIN, ARC, STG, AIT), the initial
// guess is computed analytically via the forward projection, then refined with
// Newton-Raphson iteration. This ensures convergence even at large field offsets.
//
// Returns an error if the solver does not converge within 20 iterations
// or if the Jacobian becomes singular.
func (w *WCS) WorldToPixel(world []float64) ([]float64, error) {
	if len(world) != w.nAxis {
		return nil, fmt.Errorf("%w: expected %d", ErrWCSDimension, w.nAxis)
	}

	pix := make([]float64, w.nAxis)
	proj := w.extractProjection()

	if w.nAxis >= 2 && proj != "" {
		// Analytical initial guess via forward spherical projection.
		la, lo := w.lonAxis, w.latAxis
		raRad := world[lo] * deg2rad
		decRad := world[la] * deg2rad
		alpha0 := w.crval[lo] * deg2rad
		delta0 := w.crval[la] * deg2rad

		xRad, yRad, err := project(proj, raRad, decRad, alpha0, delta0)
		if err != nil {
			// Fall back to linear guess if projection fails.
			for i := range w.nAxis {
				pix[i] = w.crpix[i] + (world[i]-w.crval[i])/w.cdelt[i]
			}
		} else {
			// Convert intermediate coords (radians) to degrees.
			xDeg := xRad * rad2deg
			yDeg := yRad * rad2deg

			// Invert the linear transform: inter = PC * (pix - crpix) * cdelt
			// First divide by cdelt to get u = PC * dp:
			u := [2]float64{xDeg / w.cdelt[lo], yDeg / w.cdelt[la]}

			// Invert 2x2 PC sub-matrix for the celestial axes:
			det := w.pc[lo][lo]*w.pc[la][la] - w.pc[lo][la]*w.pc[la][lo]
			if math.Abs(det) < 1e-30 {
				return nil, ErrWCSSingular
			}

			dp := [2]float64{
				(w.pc[la][la]*u[0] - w.pc[lo][la]*u[1]) / det,
				(w.pc[lo][lo]*u[1] - w.pc[la][lo]*u[0]) / det,
			}

			// Apply SIP inverse correction if available.
			// dp gives undistorted pixel offsets; AP/BP map back to distorted.
			if len(w.sipAP) > 0 {
				dp[0] += sipEval(w.sipAP, dp[0], dp[1])
				dp[1] += sipEval(w.sipBP, dp[0], dp[1])
			}

			pix[lo] = w.crpix[lo] + dp[0]
			pix[la] = w.crpix[la] + dp[1]

			// Higher axes: linear inverse.
			for i := range w.nAxis {
				if i != lo && i != la {
					pix[i] = w.crpix[i] + (world[i]-w.crval[i])/w.cdelt[i]
				}
			}
		}
	} else {
		// No projection: linear inverse.
		for i := range w.nAxis {
			pix[i] = w.crpix[i] + (world[i]-w.crval[i])/w.cdelt[i]
		}
	}

	const maxIter = 20

	const tol = 1e-9 // degrees (~3.6 mas — well below pixel scale, matches SCAMP fit precision)

	for range maxIter {
		fwd, err := w.PixelToWorld(pix)
		if err != nil {
			return nil, fmt.Errorf("wcs: WorldToPixel forward evaluation failed: %w", err)
		}

		// Residual in (lon, lat) world coordinates.
		lo, la := w.lonAxis, w.latAxis
		dx := fwd[lo] - world[lo]
		dy := fwd[la] - world[la]

		// Handle RA wrap-around: pick shortest arc
		if dx > 180 {
			dx -= 360
		} else if dx < -180 {
			dx += 360
		}

		if math.Abs(dx) < tol && math.Abs(dy) < tol {
			return pix, nil
		}

		// Numerical Jacobian via finite differences along pixel axes.
		const h = 1e-6

		pix1 := make([]float64, w.nAxis)
		copy(pix1, pix)
		pix1[lo] += h

		f1, err := w.PixelToWorld(pix1)
		if err != nil {
			return nil, err
		}

		pix2 := make([]float64, w.nAxis)
		copy(pix2, pix)
		pix2[la] += h

		f2, err := w.PixelToWorld(pix2)
		if err != nil {
			return nil, err
		}

		// J = [[d(lon)/dp_lo, d(lon)/dp_la], [d(lat)/dp_lo, d(lat)/dp_la]]
		j00 := (f1[lo] - fwd[lo]) / h
		j01 := (f2[lo] - fwd[lo]) / h
		j10 := (f1[la] - fwd[la]) / h
		j11 := (f2[la] - fwd[la]) / h

		// Handle RA wrap in Jacobian columns
		if j00 > 180 {
			j00 -= 360
		} else if j00 < -180 {
			j00 += 360
		}

		if j01 > 180 {
			j01 -= 360
		} else if j01 < -180 {
			j01 += 360
		}

		det := j00*j11 - j01*j10
		if math.Abs(det) < 1e-30 {
			return nil, ErrWCSSingular
		}

		// Newton update: pix -= J^{-1} * residual
		pix[lo] -= (j11*dx - j01*dy) / det
		pix[la] -= (-j10*dx + j00*dy) / det
	}

	return nil, fmt.Errorf("%w: after %d iterations", ErrWCSNotConverged, maxIter)
}

// ── Projection Helpers ───────────────────────────────────────────────────────

// extractProjection returns the 3-letter projection code from CTYPE (e.g., "TAN")
// and populates lonAxis/latAxis to record the axis mapping.
//
// Standard layout: CTYPE1="RA---TAN", CTYPE2="DEC--TAN" → lonAxis=0, latAxis=1.
// Swapped layout:  CTYPE1="DEC--TAN", CTYPE2="RA---TAN" → lonAxis=1, latAxis=0.
// SIP distortion:  CTYPE1="RA---TAN-SIP" → the "-SIP" suffix is stripped.
//
// Returns "" if no recognized celestial projection is found.
func (w *WCS) extractProjection() string {
	if w.nAxis < 2 {
		w.lonAxis, w.latAxis = -1, -1
		return ""
	}

	known := map[string]bool{"TAN": true, "SIN": true, "ARC": true, "STG": true, "AIT": true}

	// Identify which axis is longitude (RA) and which is latitude (DEC).
	var proj string

	w.lonAxis, w.latAxis = -1, -1

	for i := range 2 {
		ct := w.ctype[i]
		// Strip the "-SIP" distortion suffix if present.
		ct = strings.TrimSuffix(ct, "-SIP")

		ct = strings.TrimSuffix(ct, "-TPV")
		if len(ct) < 8 {
			continue
		}

		prefix := ct[:4]

		code := ct[5:8]
		if !known[code] {
			continue
		}

		switch {
		case strings.HasPrefix(prefix, "RA"):
			w.lonAxis = i
			proj = code
		case strings.HasPrefix(prefix, "DEC"):
			w.latAxis = i
			proj = code
		case strings.HasPrefix(prefix, "GLON") || strings.HasPrefix(prefix, "ELON"):
			w.lonAxis = i
			proj = code
		case strings.HasPrefix(prefix, "GLAT") || strings.HasPrefix(prefix, "ELAT"):
			w.latAxis = i
			proj = code
		}
	}

	// Both axes must be identified for a valid celestial WCS.
	if w.lonAxis < 0 || w.latAxis < 0 {
		w.lonAxis, w.latAxis = -1, -1
		return ""
	}

	return proj
}

// project applies the forward spherical projection to compute intermediate
// coordinates (x, y) in radians from (ra, dec) in radians relative to the
// reference point (alpha0, delta0). This is the analytical inverse of [deproject].
func project(proj string, ra, dec, alpha0, delta0 float64) (x, y float64, err error) {
	dra := ra - alpha0
	sinDRA, cosDRA := math.Sincos(dra)
	sinDec, cosDec := math.Sincos(dec)
	sinD0, cosD0 := math.Sincos(delta0)

	switch proj {
	case "TAN":
		denom := sinDec*sinD0 + cosDec*cosD0*cosDRA
		if denom <= 0 {
			return 0, 0, ErrWCSBehindPlane
		}

		x = cosDec * sinDRA / denom
		y = (sinDec*cosD0 - cosDec*sinD0*cosDRA) / denom

	case "SIN":
		x = cosDec * sinDRA
		y = sinDec*cosD0 - cosDec*sinD0*cosDRA

	case "ARC":
		sinTheta := sinDec*sinD0 + cosDec*cosD0*cosDRA
		sinTheta = math.Max(-1, math.Min(1, sinTheta))
		theta := math.Asin(sinTheta)

		r := math.Pi/2 - theta
		if r < 1e-15 {
			x, y = 0, 0
		} else {
			phi := math.Atan2(cosDec*sinDRA, sinDec*cosD0-cosDec*sinD0*cosDRA)
			x = r * math.Sin(phi)
			y = r * math.Cos(phi)
		}

	case "STG":
		sinTheta := sinDec*sinD0 + cosDec*cosD0*cosDRA
		sinTheta = math.Max(-1, math.Min(1, sinTheta))
		theta := math.Asin(sinTheta)

		r := 2 * math.Tan((math.Pi/2-theta)/2)
		if r < 1e-15 {
			x, y = 0, 0
		} else {
			phi := math.Atan2(cosDec*sinDRA, sinDec*cosD0-cosDec*sinD0*cosDRA)
			x = r * math.Sin(phi)
			y = r * math.Cos(phi)
		}

	case "AIT":
		// Hammer-Aitoff equal-area (pseudo-cylindrical, theta_0 = 0).
		//
		// Per Calabretta & Greisen (2002) §7.1, the native pole for AIT sits at
		// delta_p = delta0 + 90°, alpha_p = alpha0, phi_p = π.
		//
		// Step 1: Celestial → native spherical coordinates.
		//   sin(θ) = sin(dec)·cos(δ₀) − cos(dec)·sin(δ₀)·cos(Δα)
		//   φ = π + atan2(−cos(dec)·sin(Δα),
		//                  −sin(dec)·sin(δ₀) − cos(dec)·cos(δ₀)·cos(Δα))
		nativeTheta := math.Asin(sinDec*cosD0 - cosDec*sinD0*cosDRA)
		nativePhi := math.Pi + math.Atan2(-cosDec*sinDRA,
			-sinDec*sinD0-cosDec*cosD0*cosDRA)
		// Normalize to [-π, π] to prevent wrapping in the Hammer formula.
		nativePhi = math.Remainder(nativePhi, 2*math.Pi)

		// Step 2: Hammer projection of native coordinates.
		sinNT, cosNT := math.Sincos(nativeTheta)
		halfPhi := nativePhi / 2
		cosNTcosHalf := cosNT * math.Cos(halfPhi)

		denom := math.Sqrt(1 + cosNTcosHalf)
		if denom < 1e-15 {
			return 0, 0, ErrWCSAntipodal
		}

		s2 := math.Sqrt(2)
		x = 2 * s2 * cosNT * math.Sin(halfPhi) / denom
		y = s2 * sinNT / denom

	default:
		return 0, 0, fmt.Errorf("%w: %q", ErrWCSUnsupported, proj)
	}

	return x, y, nil
}

// deproject applies the inverse spherical projection to recover (ra, dec) in radians
// from intermediate coordinates (x, y) in radians relative to the reference point.
//
// Uses the Calabretta & Greisen (2002) conventions where for zenithal projections:
//
//	x = R(θ)·sin(φ),  y = −R(θ)·cos(φ)
//
// so sin(φ) = x/R and cos(φ) = −y/R.
func deproject(proj string, x, y, alpha0, delta0 float64) (ra, dec float64, err error) {
	sinD0, cosD0 := math.Sincos(delta0)

	switch proj {
	case "TAN":
		r := math.Hypot(x, y)
		if r == 0 {
			return alpha0, delta0, nil
		}
		// θ = atan(1/r) for gnomonic (TAN)
		theta := math.Atan(1.0 / r)
		sinT, cosT := math.Sincos(theta)
		// cos(φ) = −y/r, sin(φ) = x/r
		dec = math.Asin(sinT*sinD0 - (y/r)*cosT*cosD0)
		ra = alpha0 + math.Atan2(-x*cosT, r*sinT*cosD0+y*cosT*sinD0)

	case "SIN":
		// Orthographic (slant-projection variant with xi=0, eta=0)
		// For SIN, R(θ) = cos(θ), so x = cos(θ)sin(φ), y = −cos(θ)cos(φ)
		// sinT = sqrt(1 − r²), cosT = r = sqrt(x²+y²)
		r2 := x*x + y*y
		if r2 > 1.0 {
			return 0, 0, fmt.Errorf("%w: r²=%.6f", ErrWCSOutsideSphere, r2)
		}

		sinT := math.Sqrt(1 - r2)
		dec = math.Asin(sinT*sinD0 + y*cosD0)
		ra = alpha0 + math.Atan2(x, sinT*cosD0-y*sinD0)

	case "ARC":
		// Zenithal equidistant: R(θ) = π/2 − θ
		r := math.Hypot(x, y)
		if r == 0 {
			return alpha0, delta0, nil
		}

		theta := math.Pi/2 - r
		sinT, cosT := math.Sincos(theta)
		dec = math.Asin(sinT*sinD0 - (y/r)*cosT*cosD0)
		ra = alpha0 + math.Atan2(-x*cosT, r*sinT*cosD0+y*cosT*sinD0)

	case "STG":
		// Stereographic: R(θ) = 2·tan((π/2−θ)/2)
		r := math.Hypot(x, y)
		if r == 0 {
			return alpha0, delta0, nil
		}

		theta := math.Pi/2 - 2*math.Atan(r/2)
		sinT, cosT := math.Sincos(theta)
		dec = math.Asin(sinT*sinD0 - (y/r)*cosT*cosD0)
		ra = alpha0 + math.Atan2(-x*cosT, r*sinT*cosD0+y*cosT*sinD0)

	case "AIT":
		// Hammer-Aitoff (full-sky equal-area, pseudo-cylindrical, theta_0 = 0).
		//
		// Step 1: Inverse Hammer → native (φ, θ).
		z2 := 1.0 - (x/4)*(x/4) - (y/2)*(y/2)
		if z2 < 0 {
			return 0, 0, ErrWCSOutsideRegion
		}

		z := math.Sqrt(z2)
		nativeTheta := math.Asin(y * z)
		nativePhi := 2 * math.Atan2(x*z, 2*(2*z2-1))

		// Step 2: Native → celestial rotation.
		// For AIT with delta_p = delta0+90°, alpha_p = alpha0, phi_p = π:
		//   sin(dec) = sin(θ)·cos(δ₀) + cos(θ)·sin(δ₀)·cos(φ)
		//   ra = α₀ + atan2(cos(θ)·sin(φ),
		//                    −sin(θ)·sin(δ₀) + cos(θ)·cos(δ₀)·cos(φ))
		sinNT, cosNT := math.Sincos(nativeTheta)
		sinNP, cosNP := math.Sincos(nativePhi)
		dec = math.Asin(sinNT*cosD0 + cosNT*sinD0*cosNP)
		ra = alpha0 + math.Atan2(cosNT*sinNP,
			-sinNT*sinD0+cosNT*cosD0*cosNP)

	default:
		return 0, 0, fmt.Errorf("%w: %q", ErrWCSUnsupported, proj)
	}

	return ra, dec, nil
}

// ── FITS Header Extraction ───────────────────────────────────────────────────

// ExtractWCS dynamically translates FITS standard header layouts structurally matching
// coordinate metrics arrays natively mapping the resulting abstractions into [WCS].
func ExtractWCS(h *Header) (*WCS, error) {
	naxis, err := h.GetInt("NAXIS")
	if err != nil {
		return nil, ErrWCSMissingNAXIS
	}

	if naxis <= 0 {
		return nil, ErrWCSZeroDim
	}

	crpix := make([]float64, naxis)
	crval := make([]float64, naxis)
	cdelt := make([]float64, naxis)
	ctype := make([]string, naxis)
	pc := make([][]float64, naxis)

	for i := 1; i <= naxis; i++ {
		idx := i - 1

		c, _ := h.GetString(fmt.Sprintf("CTYPE%d", i))
		ctype[idx] = strings.TrimSpace(c)

		if v, err := h.GetFloat(fmt.Sprintf("CRVAL%d", i)); err == nil {
			crval[idx] = v
		}

		if p, err := h.GetFloat(fmt.Sprintf("CRPIX%d", i)); err == nil {
			crpix[idx] = p
		}

		d, err := h.GetFloat(fmt.Sprintf("CDELT%d", i))
		if err != nil {
			d = 1.0
		}

		cdelt[idx] = d

		pc[idx] = make([]float64, naxis)
	}

	// Try CDi_j first (used by HST, JWST, DES, LSST, and most survey pipelines).
	// If any CDi_j keyword is found, use the CD matrix convention exclusively.
	hasCDMatrix := false

	cd := make([][]float64, naxis)
	for i := 1; i <= naxis; i++ {
		cd[i-1] = make([]float64, naxis)
		for j := 1; j <= naxis; j++ {
			val, err := h.GetFloat(fmt.Sprintf("CD%d_%d", i, j))
			if err == nil {
				hasCDMatrix = true
				cd[i-1][j-1] = val
			}
		}
	}

	if hasCDMatrix {
		// Decompose CD matrix: CDi_j = CDELTi * PCi_j
		// Extract CDELT as column norms and PC as the normalized rotation.
		for i := range naxis {
			var colNorm float64
			for j := range naxis {
				colNorm += cd[j][i] * cd[j][i]
			}

			colNorm = math.Sqrt(colNorm)
			if colNorm == 0 {
				colNorm = 1.0 // Avoid division by zero for degenerate axes
			}

			// Preserve sign from the diagonal element
			if cd[i][i] < 0 {
				cdelt[i] = -colNorm
			} else {
				cdelt[i] = colNorm
			}

			for j := range naxis {
				pc[j][i] = cd[j][i] / cdelt[i]
			}
		}
	} else {
		// Fall back to PCi_j + CDELT convention.
		for i := 1; i <= naxis; i++ {
			for j := 1; j <= naxis; j++ {
				val, err := h.GetFloat(fmt.Sprintf("PC%d_%d", i, j))
				if err == nil {
					pc[i-1][j-1] = val
				} else if i == j {
					pc[i-1][j-1] = 1.0
				}
			}
		}
	}

	w := NewWCS(naxis)
	w.SetCTYPE(ctype)
	w.SetCRVAL(crval)
	w.SetCRPIX(crpix)
	w.SetCDELT(cdelt)
	w.SetPC(pc)

	// Extract SIP distortion coefficients if present.
	sipA := parseSIPPoly(h, "A")

	sipB := parseSIPPoly(h, "B")
	if len(sipA) > 0 || len(sipB) > 0 {
		w.SetSIP(sipA, sipB)
	}

	sipAP := parseSIPPoly(h, "AP")

	sipBP := parseSIPPoly(h, "BP")
	if len(sipAP) > 0 || len(sipBP) > 0 {
		w.SetSIPInverse(sipAP, sipBP)
	}

	// Extract TPV distortion coefficients if CTYPE contains "-TPV" suffix.
	hasTPV := false

	for _, ct := range ctype {
		if strings.HasSuffix(ct, "-TPV") {
			hasTPV = true
			break
		}
	}

	if hasTPV {
		pv1 := parseTPVCoeffs(h, 1)

		pv2 := parseTPVCoeffs(h, 2)
		if len(pv1) > 0 || len(pv2) > 0 {
			w.SetTPV(pv1, pv2)
		}
	}

	return w, nil
}

// parseSIPPoly reads a SIP polynomial from a FITS header.
// prefix is one of "A", "B", "AP", "BP".
// Returns nil if the ORDER keyword is not found.
func parseSIPPoly(h *Header, prefix string) map[[2]int]float64 {
	order, err := h.GetInt(prefix + "_ORDER")
	if err != nil || order < 0 {
		return nil
	}

	coeffs := make(map[[2]int]float64)

	for p := 0; p <= order; p++ {
		for q := 0; q <= order-p; q++ {
			key := fmt.Sprintf("%s_%d_%d", prefix, p, q)
			if v, err := h.GetFloat(key); err == nil && v != 0 {
				coeffs[[2]int{p, q}] = v
			}
		}
	}

	return coeffs
}

// tpvEval evaluates a TPV distortion polynomial at intermediate coordinates (x, y).
// The TPV convention defines up to 40 terms (indices 0–39) using the standard
// SCAMP/SExtractor polynomial ordering. Term index maps to:
//
//	 0: 1            1: x            2: y            3: r
//	 4: x²           5: xy           6: y²           7: x³
//	 8: x²y          9: xy²         10: y³          11: r³
//	12: x⁴          13: x³y         14: x²y²        15: xy³
//	16: y⁴          17: x⁵          18: x⁴y         19: x³y²
//	20: x²y³        21: xy⁴         22: y⁵          23: r⁵
//	24: x⁶          25: x⁵y         26: x⁴y²        27: x³y³
//	28: x²y⁴        29: xy⁵         30: y⁶          31: x⁷
//	32: x⁶y         33: x⁵y²        34: x⁴y³        35: x³y⁴
//	36: x²y⁵        37: xy⁶         38: y⁷          39: r⁷
func tpvEval(coeffs map[int]float64, x, y float64) float64 {
	if len(coeffs) == 0 {
		return 0 // empty polynomial — caller should guard with len() > 0
	}

	x2 := x * x
	y2 := y * y
	xy := x * y
	r2 := x2 + y2
	r := math.Sqrt(r2)

	// Build the term table. Only compute terms that have nonzero coefficients.
	terms := [40]float64{
		0:  1,
		1:  x,
		2:  y,
		3:  r,
		4:  x2,
		5:  xy,
		6:  y2,
		7:  x2 * x,
		8:  x2 * y,
		9:  x * y2,
		10: y2 * y,
		11: r2 * r,
		12: x2 * x2,
		13: x2 * xy,
		14: x2 * y2,
		15: xy * y2,
		16: y2 * y2,
		17: x2 * x2 * x,
		18: x2 * x2 * y,
		19: x2 * x * y2,
		20: x2 * y2 * y,
		21: x * y2 * y2,
		22: y2 * y2 * y,
		23: r2 * r2 * r,
		24: x2 * x2 * x2,
		25: x2 * x2 * xy,
		26: x2 * x2 * y2,
		27: x2 * x * y2 * y,
		28: x2 * y2 * y2,
		29: x * y2 * y2 * y,
		30: y2 * y2 * y2,
		31: x2 * x2 * x2 * x,
		32: x2 * x2 * x2 * y,
		33: x2 * x2 * x * y2,
		34: x2 * x2 * y2 * y,
		35: x2 * x * y2 * y2,
		36: x2 * y2 * y2 * y,
		37: x * y2 * y2 * y2,
		38: y2 * y2 * y2 * y,
		39: r2 * r2 * r2 * r,
	}

	var sum float64

	for idx, c := range coeffs {
		if idx >= 0 && idx < 40 {
			sum += c * terms[idx]
		}
	}

	return sum
}

// parseTPVCoeffs reads TPV polynomial coefficients from a FITS header.
// axis is 1 or 2 (for PV1_j or PV2_j keywords).
// Returns nil if no PV keywords are found.
func parseTPVCoeffs(h *Header, axis int) map[int]float64 {
	coeffs := make(map[int]float64)

	for j := range 40 {
		key := fmt.Sprintf("PV%d_%d", axis, j)
		if v, err := h.GetFloat(key); err == nil {
			coeffs[j] = v
		}
	}

	if len(coeffs) == 0 {
		return nil
	}

	return coeffs
}
