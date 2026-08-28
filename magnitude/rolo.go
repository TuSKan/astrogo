package magnitude

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/unit"
)

// ── ROLO lunar disk-equivalent reflectance ───────────────────────────────────

// Sentinel errors for the ROLO model.
var (
	// ErrROLOBandCount is returned when a destination slice does not have
	// one slot per ROLO band.
	ErrROLOBandCount = errors.New("magnitude: need one value per ROLO band")

	// ErrROLOPhaseRange reports a phase angle outside the range the model
	// was fitted over. The reflectance is still computed and returned; the
	// error says the result is an extrapolation.
	ErrROLOPhaseRange = errors.New("magnitude: phase angle outside the ROLO fitted range")

	// ErrROLODistance is returned for a non-positive distance.
	ErrROLODistance = errors.New("magnitude: distance must be positive")
)

// ROLO model reference values and their provenance.
//
//   - Model: ROLO (RObotic Lunar Observatory) lunar irradiance model,
//     version 311g.
//   - Primary reference: Kieffer, H.H. & Stone, T.C. (2005), "The spectral
//     irradiance of the Moon", AJ 129, 2887, doi:10.1086/430185.
//   - Equations implemented: Eq. 10 (disk-equivalent reflectance).
//   - Coefficients: Table 4 (32 bands x 10 wavelength-dependent terms) and
//     Table 5 (8 wavelength-independent terms).
//   - Units: reflectance dimensionless; see [ROLOGeometry] for the angles,
//     which do not all share one unit.
const (
	// ROLOSolidAngleSR is the solid angle subtended by the Moon at the
	// standard Moon-observer distance, in steradians.
	ROLOSolidAngleSR = 6.4177e-5

	// ROLOStandardDistanceKM is the Moon-observer distance the model
	// normalises to, in kilometres.
	ROLOStandardDistanceKM = 384400.0

	// ROLOMinPhaseDeg and ROLOMaxPhaseDeg bound the absolute phase angle
	// over which the model was fitted. Below the minimum the Moon is inside
	// Earth's shadow cone or close to it, and no ROLO observation exists;
	// above the maximum the illuminated crescent is too thin for the
	// disk-equivalent formulation.
	ROLOMinPhaseDeg = 1.55
	ROLOMaxPhaseDeg = 97.0

	// ROLOFitResidual is the published RMS residual of the 311g fit, in
	// natural-log reflectance — roughly 1% in reflectance. It is the model's
	// own internal consistency, not its absolute accuracy, which Kieffer &
	// Stone put near 5-10% and which no term in this file can improve.
	ROLOFitResidual = 0.0096
)

// The four libration and four nonlinear-phase coefficients, which Kieffer &
// Stone (2005) Table 5 marks with an asterisk and its NOTE states are
// "constant for all wavelengths". They are absent from Table 4 for exactly
// that reason: Table 4 tabulates only the terms that vary by band.
//
// Table 5 is headed "Example Lunar Irradiance Model Coefficients" and its
// wavelength-dependent values do not match any Table 4 row, so it shows a
// different fit; only its asterisked entries are used here, on the strength
// of that NOTE. Table 5 also prints c1-c4 to one or two significant figures
// against stated effects on ln A of 0.005-0.028, so the rounding is a
// material fraction of the libration terms' own contribution. Those terms
// are the four smallest in the model, but the caveat belongs in any error
// budget built on this code.
const (
	roloC1 = 0.0003  // libration in latitude, deg^-1
	roloC2 = -0.0013 // libration in longitude, deg^-1
	roloC3 = 0.0010  // solar longitude x libration latitude, deg^-1 rad^-1
	roloC4 = 0.0006  // solar longitude x libration longitude, deg^-1 rad^-1

	roloP1 = 4.06   // first exponential scale, deg
	roloP2 = 12.88  // second exponential scale, deg
	roloP3 = -30.59 // cosine phase offset, deg
	roloP4 = 16.75  // cosine period, deg
)

// roloBand holds one row of Kieffer & Stone (2005) Table 4: the band centre
// and its ten wavelength-dependent coefficients.
type roloBand struct {
	Lambda unit.WavelengthNM
	A      [4]float64 // a0..a3, phase polynomial, rad^-i
	B      [3]float64 // b1..b3, odd powers of solar longitude, rad^-(2j-1)
	D      [3]float64 // d1..d3, two exponentials and a cosine in phase
}

// roloBands is Kieffer & Stone (2005) Table 4 in full, model version 311g.
//
// The 2126.3 nm row is not a transcription error: its a1 and a2 depart
// sharply from every neighbouring band (-2.55069 and 2.10026 against roughly
// -1.3 and 0.2), and its d3 is an order of magnitude larger. That band sits
// in a strong telluric absorption region, and the fit absorbs the resulting
// poor sampling into the phase terms. The values are as published, verified
// against both the journal's rendered table and its ASCII export.
var roloBands = [32]roloBand{
	{350.0, [4]float64{-2.67511, -1.78539, 0.50612, -0.25578}, [3]float64{0.03744, 0.00981, -0.00322}, [3]float64{0.34185, 0.01441, -0.01602}},
	{355.1, [4]float64{-2.71924, -1.74298, 0.44523, -0.23315}, [3]float64{0.03492, 0.01142, -0.00383}, [3]float64{0.33875, 0.01612, -0.00996}},
	{405.0, [4]float64{-2.35754, -1.72134, 0.40337, -0.21105}, [3]float64{0.03505, 0.01043, -0.00341}, [3]float64{0.35235, -0.03818, -0.00006}},
	{412.3, [4]float64{-2.34185, -1.74337, 0.42156, -0.21512}, [3]float64{0.03141, 0.01364, -0.00472}, [3]float64{0.36591, -0.05902, 0.00080}},
	{414.4, [4]float64{-2.43367, -1.72184, 0.43600, -0.22675}, [3]float64{0.03474, 0.01188, -0.00422}, [3]float64{0.35558, -0.03247, -0.00503}},
	{441.6, [4]float64{-2.31964, -1.72114, 0.37286, -0.19304}, [3]float64{0.03736, 0.01545, -0.00559}, [3]float64{0.37935, -0.09562, 0.00970}},
	{465.8, [4]float64{-2.35085, -1.66538, 0.41802, -0.22541}, [3]float64{0.04274, 0.01127, -0.00439}, [3]float64{0.33450, -0.02546, -0.00484}},
	{475.0, [4]float64{-2.28999, -1.63180, 0.36193, -0.20381}, [3]float64{0.04007, 0.01216, -0.00437}, [3]float64{0.33024, -0.03131, 0.00222}},
	{486.9, [4]float64{-2.23351, -1.68573, 0.37632, -0.19877}, [3]float64{0.03881, 0.01566, -0.00555}, [3]float64{0.36590, -0.08945, 0.00678}},
	{544.0, [4]float64{-2.13864, -1.60613, 0.27886, -0.16426}, [3]float64{0.03833, 0.01189, -0.00390}, [3]float64{0.37190, -0.10629, 0.01428}},
	{549.1, [4]float64{-2.10782, -1.66736, 0.41697, -0.22026}, [3]float64{0.03451, 0.01452, -0.00517}, [3]float64{0.36814, -0.09815, -0.00000}},
	{553.8, [4]float64{-2.12504, -1.65970, 0.38409, -0.20655}, [3]float64{0.04052, 0.01009, -0.00388}, [3]float64{0.37206, -0.10745, 0.00347}},
	{665.1, [4]float64{-1.88914, -1.58096, 0.30477, -0.17908}, [3]float64{0.04415, 0.00983, -0.00389}, [3]float64{0.37141, -0.13514, 0.01248}},
	{693.1, [4]float64{-1.89410, -1.58509, 0.28080, -0.16427}, [3]float64{0.04429, 0.00914, -0.00351}, [3]float64{0.39109, -0.17048, 0.01754}},
	{703.6, [4]float64{-1.92103, -1.60151, 0.36924, -0.20567}, [3]float64{0.04494, 0.00987, -0.00386}, [3]float64{0.37155, -0.13989, 0.00412}},
	{745.3, [4]float64{-1.86896, -1.57522, 0.33712, -0.19415}, [3]float64{0.03967, 0.01318, -0.00464}, [3]float64{0.36888, -0.14828, 0.00958}},
	{763.7, [4]float64{-1.85258, -1.47181, 0.14377, -0.11589}, [3]float64{0.04435, 0.02000, -0.00738}, [3]float64{0.39126, -0.16957, 0.03053}},
	{774.8, [4]float64{-1.80271, -1.59357, 0.36351, -0.20326}, [3]float64{0.04710, 0.01196, -0.00476}, [3]float64{0.36908, -0.16182, 0.00830}},
	{865.3, [4]float64{-1.74561, -1.58482, 0.35009, -0.19569}, [3]float64{0.04142, 0.01612, -0.00550}, [3]float64{0.39200, -0.18837, 0.00978}},
	{872.6, [4]float64{-1.76779, -1.60345, 0.37974, -0.20625}, [3]float64{0.04645, 0.01170, -0.00424}, [3]float64{0.39354, -0.19360, 0.00568}},
	{882.0, [4]float64{-1.73011, -1.61156, 0.36115, -0.19576}, [3]float64{0.04847, 0.01065, -0.00404}, [3]float64{0.40714, -0.21499, 0.01146}},
	{928.4, [4]float64{-1.75981, -1.45395, 0.13780, -0.11254}, [3]float64{0.05000, 0.01476, -0.00513}, [3]float64{0.41900, -0.19963, 0.02940}},
	{939.3, [4]float64{-1.76245, -1.49892, 0.07956, -0.07546}, [3]float64{0.05461, 0.01355, -0.00464}, [3]float64{0.47936, -0.29463, 0.04706}},
	{942.1, [4]float64{-1.66473, -1.61875, 0.14630, -0.09216}, [3]float64{0.04533, 0.03010, -0.01166}, [3]float64{0.57275, -0.38204, 0.04902}},
	{1059.5, [4]float64{-1.59323, -1.71358, 0.50599, -0.25178}, [3]float64{0.04906, 0.03178, -0.01138}, [3]float64{0.48160, -0.29486, 0.00116}},
	{1243.2, [4]float64{-1.53594, -1.55214, 0.31479, -0.18178}, [3]float64{0.03965, 0.03009, -0.01123}, [3]float64{0.49040, -0.30970, 0.01237}},
	{1538.7, [4]float64{-1.33802, -1.46208, 0.15784, -0.11712}, [3]float64{0.04674, 0.01471, -0.00656}, [3]float64{0.53831, -0.38432, 0.03473}},
	{1633.6, [4]float64{-1.34567, -1.46057, 0.23813, -0.15494}, [3]float64{0.03883, 0.02280, -0.00877}, [3]float64{0.54393, -0.37182, 0.01845}},
	{1981.5, [4]float64{-1.26203, -1.25138, -0.06569, -0.04005}, [3]float64{0.04157, 0.02036, -0.00772}, [3]float64{0.49099, -0.36092, 0.04707}},
	{2126.3, [4]float64{-1.18946, -2.55069, 2.10026, -0.87285}, [3]float64{0.03819, -0.00685, -0.00200}, [3]float64{0.29239, -0.34784, -0.13444}},
	{2250.9, [4]float64{-1.04232, -1.46809, 0.43817, -0.24632}, [3]float64{0.04893, 0.00617, -0.00259}, [3]float64{0.38154, -0.28937, -0.01110}},
	{2383.6, [4]float64{-1.08403, -1.31032, 0.20323, -0.15863}, [3]float64{0.05955, -0.00940, 0.00083}, [3]float64{0.36134, -0.28408, 0.01010}},
}

// ROLOBands returns the 32 band centres of the ROLO model, in nanometres and
// ascending order.
//
// The model is defined only at these wavelengths. It is a fit to broadband
// photometry in 32 filters, not a continuous spectrum, so a value between two
// bands is the caller's interpolation and carries the caller's assumption —
// which matters most across 943 nm and 1400 nm, where the neighbouring bands
// straddle telluric water absorption and the true lunar spectrum is not
// smooth between them.
func ROLOBands() []unit.WavelengthNM {
	out := make([]unit.WavelengthNM, len(roloBands))
	for i, b := range roloBands {
		out[i] = b.Lambda
	}

	return out
}

// ROLOGeometry is the Sun-Moon-observer geometry the ROLO model needs.
//
// The four angles do not share a unit in the published model — the phase
// polynomial is in radians, the exponential and cosine terms in degrees, the
// solar longitude in radians and the libration angles in degrees — which is
// why they are [angle.Angle] here rather than float64. Passing a number in
// the wrong unit is the single most likely way to get a plausible but wrong
// answer out of this model.
type ROLOGeometry struct {
	// PhaseAngle is the absolute Sun-Moon-observer angle. Its sign is
	// discarded; the waxing/waning asymmetry is carried by SolarLongitude,
	// which is what makes the model directional rather than symmetric about
	// full Moon. [PhaseAngle] computes it from an ephemeris.
	PhaseAngle angle.Angle

	// SolarLongitude is the selenographic longitude of the Sun, signed:
	// negative before full Moon and positive after, following Kieffer &
	// Stone's convention. Because the Moon's prime meridian points at Earth
	// to within libration, it is close to the signed phase angle, but it is
	// not identical and the difference is what the b terms resolve.
	SolarLongitude angle.Angle

	// LibrationLatitude and LibrationLongitude are the selenographic
	// coordinates of the sub-observer point: which way the Moon is tipped
	// toward the observer. Zero is the mean sub-Earth point.
	//
	// These are the four smallest terms in the model. Leaving them zero
	// costs at most about 0.03 in ln A by Table 5's own accounting, well
	// inside the model's absolute accuracy, so a caller without lunar
	// orientation data can pass zeroes and record the omission.
	LibrationLatitude  angle.Angle
	LibrationLongitude angle.Angle
}

// ROLOReflectance writes the disk-equivalent reflectance A of the Moon at
// each of the 32 [ROLOBands] into dst, which must have that many slots.
//
// Kieffer & Stone (2005) Eq. 10:
//
//	ln A_k = sum(i=0..3) a_ik g^i + sum(j=1..3) b_jk P^(2j-1)
//	         + c1 t + c2 p + c3 P t + c4 P p
//	         + d1k exp(-g/p1) + d2k exp(-g/p2) + d3k cos((g-p3)/p4)
//
// with g the absolute phase angle, P the selenographic longitude of the Sun,
// and t, p the libration latitude and longitude. **g appears in two different
// units**: radians in the polynomial, degrees in the exponential and cosine
// terms, because p1..p4 are published in degrees. Both are applied here.
//
// Disk-equivalent reflectance is the reflectance a Lambertian disk of the
// Moon's angular size would need to produce the observed irradiance. It is
// not a surface albedo and is not resolved across the disk — the whole point
// of the formulation is that it needs no lunar surface map.
//
// A phase angle outside [ROLOMinPhaseDeg], [ROLOMaxPhaseDeg] still produces
// values, together with an [ErrROLOPhaseRange] error saying they are an
// extrapolation. The most common case is a nearly full Moon, where the model
// is being asked about the opposition surge just inside the range it saw.
func ROLOReflectance(dst []float64, geom ROLOGeometry) error {
	if len(dst) != len(roloBands) {
		return fmt.Errorf("%w: got %d slots, want %d", ErrROLOBandCount, len(dst), len(roloBands))
	}

	gRad := math.Abs(geom.PhaseAngle.Radians())
	gDeg := math.Abs(geom.PhaseAngle.Degrees())

	if math.IsNaN(gRad) {
		return fmt.Errorf("%w: phase angle is not a number", ErrROLOPhaseRange)
	}

	pRad := geom.SolarLongitude.Radians()
	tDeg := geom.LibrationLatitude.Degrees()
	pDeg := geom.LibrationLongitude.Degrees()

	// The libration group and the two phase nonlinearities are the same for
	// every band, so they are evaluated once.
	libration := roloC1*tDeg + roloC2*pDeg + roloC3*pRad*tDeg + roloC4*pRad*pDeg
	exp1 := math.Exp(-gDeg / roloP1)
	exp2 := math.Exp(-gDeg / roloP2)
	cosTerm := math.Cos((gDeg - roloP3) / roloP4)

	// Odd powers of the solar longitude: P, P^3, P^5.
	p1, p3, p5 := pRad, pRad*pRad*pRad, pRad*pRad*pRad*pRad*pRad

	for i, b := range roloBands {
		lnA := b.A[0] + b.A[1]*gRad + b.A[2]*gRad*gRad + b.A[3]*gRad*gRad*gRad
		lnA += b.B[0]*p1 + b.B[1]*p3 + b.B[2]*p5
		lnA += libration
		lnA += b.D[0]*exp1 + b.D[1]*exp2 + b.D[2]*cosTerm

		dst[i] = math.Exp(lnA)
	}

	if gDeg < ROLOMinPhaseDeg || gDeg > ROLOMaxPhaseDeg {
		return fmt.Errorf("%w: %.3f deg is outside [%.2f, %.2f]",
			ErrROLOPhaseRange, gDeg, ROLOMinPhaseDeg, ROLOMaxPhaseDeg)
	}

	return nil
}

// ROLOIrradiance converts disk-equivalent reflectance into the lunar spectral
// irradiance seen by an observer, given the solar spectral irradiance at 1 AU
// on the same wavelength grid.
//
// Unlike [ROLOReflectance] this is not transcribed from the paper — it is the
// definition of disk-equivalent reflectance run backwards, plus inverse-square
// geometry. A Lambertian disk of reflectance A under irradiance E_sun has
// radiance A E_sun / pi, and a source of solid angle Omega delivers
// irradiance L Omega, so
//
//	E = A E_sun Omega_M / pi
//
// at the standard distances, scaled by (1 AU / d_sun)^2 for the Sun-Moon leg
// and ([ROLOStandardDistanceKM] / d_moon)^2 for the Moon-observer leg.
//
// The solar spectrum is a parameter rather than a table because this package
// ships none: the ROLO model's own absolute scale depends on which solar
// irradiance reference it is paired with, and quietly picking one would hide
// a choice that belongs to the caller. dst may alias reflectance.
func ROLOIrradiance(dst, reflectance, solar []float64, sunDistanceAU, moonDistanceKM float64) error {
	if len(dst) != len(reflectance) || len(dst) != len(solar) {
		return fmt.Errorf("%w: %d destination, %d reflectance, %d solar",
			ErrROLOBandCount, len(dst), len(reflectance), len(solar))
	}

	if sunDistanceAU <= 0 || moonDistanceKM <= 0 || math.IsNaN(sunDistanceAU) || math.IsNaN(moonDistanceKM) {
		return fmt.Errorf("%w: sun %g AU, moon %g km", ErrROLODistance, sunDistanceAU, moonDistanceKM)
	}

	sunScale := 1 / (sunDistanceAU * sunDistanceAU)
	moonScale := (ROLOStandardDistanceKM / moonDistanceKM) * (ROLOStandardDistanceKM / moonDistanceKM)
	factor := ROLOSolidAngleSR / math.Pi * sunScale * moonScale

	for i := range dst {
		dst[i] = reflectance[i] * solar[i] * factor
	}

	return nil
}
