package skybrightness

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/unit"
)

// ErrZodiacalGeometry is returned for a direction the tabulated map does not
// cover, or for a non-physical heliocentric distance.
var ErrZodiacalGeometry = errors.New("skybrightness: zodiacal geometry outside the tabulated map")

// Zodiacal light — sunlight scattered by interplanetary dust.
//
//   - Model: Leinert et al. (1998) Table 17 for the spatial distribution at
//     500 nm and Eq. 22 for the colour correction, with the heliocentric and
//     seasonal factors applied as Masana et al. (2021) Eq. 18 does.
//   - Primary reference: Leinert, Ch. et al. (1998), A&AS 127, 1, "The 1997
//     reference of diffuse night sky brightness".
//
// It is a significant term at low ecliptic latitude and cannot be treated as
// a constant floor: the tabulated map spans a factor of 160 between the
// ecliptic pole and the region nearest the Sun.
const (
	// ZodiacalReferenceNM is the wavelength the tabulated map is given at.
	ZodiacalReferenceNM = 500.0

	// ZodiacalPoleBrightness is the brightness toward the ecliptic pole in
	// the table's own units, quoted separately by Leinert et al. because the
	// grid itself stops at 75 degrees.
	ZodiacalPoleBrightness = 77.0

	// zodiacalTableUnit converts the table's 1e-8 W m^-2 sr^-1 um^-1 into
	// W m^-2 sr^-1 nm^-1: 1e-8 for the prefix, then per-micron to
	// per-nanometre.
	zodiacalTableUnit = 1e-8 / 1000

	// zodiacalHeliocentricExponent is the power of heliocentric distance the
	// visual brightness follows, after Leinert et al. (1980).
	zodiacalHeliocentricExponent = -2.3

	// zodiacalNodeLongitude is the ecliptic longitude of the ascending node
	// of the dust cloud's plane of symmetry, in degrees. The Earth's
	// excursion above and below that plane is what makes the high-latitude
	// brightness vary through the year.
	zodiacalNodeLongitude = 96.0

	// zodiacalSeasonalAmplitude is the fractional variation that excursion
	// produces, applied only above zodiacalSeasonalLatitude.
	zodiacalSeasonalAmplitude = 0.1
	zodiacalSeasonalLatitude  = 60.0

	// The span over which Leinert et al. give the colour correction.
	zodiacalColourMinNM = 220.0
	zodiacalColourMaxNM = 2500.0
)

// zodiacalLongitudes are the differential ecliptic longitudes — the viewing
// direction's minus the Sun's — indexing the rows of Leinert Table 17.
var zodiacalLongitudes = [19]float64{
	0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 60, 75, 90, 105, 120, 135, 150, 165, 180,
}

// zodiacalLatitudes are the ecliptic latitudes indexing the columns.
var zodiacalLatitudes = [10]float64{0, 5, 10, 15, 20, 25, 30, 45, 60, 75}

// zodiacalMissing marks the solar vicinity, within roughly 15 degrees
// elongation, which Table 17 does not cover.
var zodiacalMissing = math.NaN()

// zodiacalBrightness is Leinert et al. (1998) Table 17: zodiacal light
// observed from Earth at 0.50 um, in 1e-8 W m^-2 sr^-1 um^-1.
//
// The blank entries are the solar vicinity. They are not zeroes and not
// interpolable — the brightness there rises by another order of magnitude —
// so a direction resolving to one is an error rather than a number.
var zodiacalBrightness = [19][10]float64{
	{zodiacalMissing, zodiacalMissing, zodiacalMissing, 3140, 1610, 985, 640, 275, 150, 100},
	{zodiacalMissing, zodiacalMissing, zodiacalMissing, 2940, 1540, 945, 625, 271, 150, 100},
	{zodiacalMissing, zodiacalMissing, 4740, 2470, 1370, 865, 590, 264, 148, 100},
	{11500, 6780, 3440, 1860, 1110, 755, 525, 251, 146, 100},
	{6400, 4480, 2410, 1410, 910, 635, 454, 237, 141, 99},
	{3840, 2830, 1730, 1100, 749, 545, 410, 223, 136, 97},
	{2480, 1870, 1220, 845, 615, 467, 365, 207, 131, 95},
	{1650, 1270, 910, 680, 510, 397, 320, 193, 125, 93},
	{1180, 940, 700, 530, 416, 338, 282, 179, 120, 92},
	{910, 730, 555, 442, 356, 292, 250, 166, 116, 90},
	{505, 442, 352, 292, 243, 209, 183, 134, 104, 86},
	{338, 317, 269, 227, 196, 172, 151, 116, 93, 82},
	{259, 251, 225, 193, 166, 147, 132, 104, 86, 79},
	{212, 210, 197, 170, 150, 133, 119, 96, 82, 77},
	{188, 186, 177, 154, 138, 125, 113, 90, 77, 74},
	{179, 178, 166, 147, 134, 122, 110, 90, 77, 73},
	{179, 178, 165, 148, 137, 127, 116, 96, 79, 72},
	{196, 192, 179, 165, 151, 141, 131, 104, 82, 72},
	{230, 212, 195, 178, 163, 148, 134, 105, 83, 72},
}

// ZodiacalGeometry locates a line of sight relative to the Sun and the
// ecliptic, and the observer relative to the dust cloud.
type ZodiacalGeometry struct {
	// DifferentialLongitude is the viewing direction's ecliptic longitude
	// minus the Sun's. Its sign does not matter — the cloud is symmetric
	// about the Sun-Earth line — and it is folded into [0, 180].
	DifferentialLongitude angle.Angle

	// EclipticLatitude is the viewing direction's, folded into [0, 90] for
	// the same reason.
	EclipticLatitude angle.Angle

	// SunDistanceAU is the observer's heliocentric distance. Earth's orbital
	// eccentricity moves it by about 3 per cent over a year, and the
	// brightness with the -2.3 power of it.
	SunDistanceAU float64

	// EarthLongitude is the Earth's own ecliptic longitude, which sets where
	// in its excursion out of the dust plane the observer is.
	EarthLongitude angle.Angle
}

// ZodiacalBrightnessAt returns the 500 nm zodiacal brightness in
// W m^-2 sr^-1 nm^-1, bilinearly interpolated in Leinert et al. (1998)
// Table 17.
//
// Past 75 degrees of ecliptic latitude the grid stops and the separately
// quoted pole value is interpolated toward, which is why the pole is a named
// constant rather than another column.
//
// Directions inside the solar vicinity the table excludes return
// [ErrZodiacalGeometry]. That is deliberate: extrapolating there means
// guessing at a region where the brightness climbs by another order of
// magnitude, and a night-sky model has no business reporting a number a few
// degrees from the Sun.
func ZodiacalBrightnessAt(geom ZodiacalGeometry) (float64, error) {
	lon := foldDifferentialLongitude(geom.DifferentialLongitude)

	lat := foldEclipticLatitude(geom.EclipticLatitude)

	table, err := interpolateZodiacal(lon, lat)
	if err != nil {
		return 0, err
	}

	return table * zodiacalTableUnit, nil
}

// foldDifferentialLongitude maps onto [0, 180], where the table is defined;
// the dust cloud is symmetric about the Sun-Earth line.
//
// The fold is done in degrees rather than through angle.Wrap180 so that a
// direction given a full turn away lands on exactly the same table cell. Via
// radians it lands one ulp off, which is invisible everywhere except on a
// grid line, where it silently picks the neighbouring interval.
func foldDifferentialLongitude(a angle.Angle) float64 {
	d := math.Mod(a.Degrees(), 360)
	if d < 0 {
		d += 360
	}

	if d > 180 {
		d = 360 - d
	}

	return d
}

// foldEclipticLatitude maps onto [0, 90], in degrees for the same reason.
func foldEclipticLatitude(a angle.Angle) float64 {
	d := math.Mod(a.Degrees(), 360)
	if d < 0 {
		d += 360
	}

	if d > 180 {
		d = 360 - d
	}

	if d > 90 {
		d = 180 - d
	}

	return d
}

// interpolateZodiacal does the bilinear lookup in the table's own units.
func interpolateZodiacal(lon, lat float64) (float64, error) {
	i, fi := bracketAxis(zodiacalLongitudes[:], lon)
	j, fj := bracketZodiacalLatitude(lat)

	// Weight first, then read. A direction sitting exactly on a grid line at
	// the edge of the excluded solar vicinity has a missing neighbour with
	// zero weight, and refusing it because that neighbour is blank would
	// reject perfectly good tabulated points — the brightest cell in the
	// table among them.
	corners := [4]struct {
		value, weight float64
	}{
		{zodiacalCell(i, j), (1 - fi) * (1 - fj)},
		{zodiacalCell(i, j+1), (1 - fi) * fj},
		{zodiacalCell(i+1, j), fi * (1 - fj)},
		{zodiacalCell(i+1, j+1), fi * fj},
	}

	var total float64

	for _, c := range corners {
		if c.weight == 0 {
			continue
		}

		if math.IsNaN(c.value) {
			return 0, fmt.Errorf("%w: %.1f deg from the Sun in longitude, %.1f deg ecliptic latitude",
				ErrZodiacalGeometry, lon, lat)
		}

		total += c.value * c.weight
	}

	return total, nil
}

// zodiacalCell reads the table, standing in the separately quoted pole
// brightness for the column past the tabulated 75 degrees.
func zodiacalCell(i, j int) float64 {
	if j >= len(zodiacalLatitudes) {
		return ZodiacalPoleBrightness
	}

	return zodiacalBrightness[i][j]
}

// gridSnapTolerance is how close to an interval's end a position must be to
// count as sitting exactly on it.
//
// An angle.Angle holds radians, so a direction given in degrees round-trips
// through a conversion and lands within an ulp or two of where it started:
// angle.Deg(15).Degrees() is 14.999999999999998. Without snapping, a point on
// a grid line gets a weight of 4e-16 on the neighbouring cell — harmless for
// an ordinary interpolation, fatal at the edge of the table's blank region,
// where that neighbour is missing and a vanishing weight would still veto the
// lookup. It cost the brightest cell in the table.
//
// The tolerance is a fraction of an interval, so on this 5-degree grid it
// corresponds to well under a microdegree: far below any real pointing and
// far above the round-trip noise.
const gridSnapTolerance = 1e-9

// bracketAxis finds the interval of axis containing v and the fraction along
// it, clamping at both ends and snapping to a grid line within
// [gridSnapTolerance].
func bracketAxis(axis []float64, v float64) (int, float64) {
	last := len(axis) - 1

	switch {
	case v <= axis[0]:
		return 0, 0
	case v >= axis[last]:
		return last - 1, 1
	}

	for i := 1; i <= last; i++ {
		if v > axis[i] {
			continue
		}

		f := (v - axis[i-1]) / (axis[i] - axis[i-1])

		switch {
		case f < gridSnapTolerance:
			f = 0
		case f > 1-gridSnapTolerance:
			f = 1
		}

		return i - 1, f
	}

	return last - 1, 1
}

// bracketZodiacalLatitude brackets the latitude axis, extending it to the
// pole with the separately quoted value.
func bracketZodiacalLatitude(lat float64) (int, float64) {
	last := len(zodiacalLatitudes) - 1
	if lat >= zodiacalLatitudes[last] {
		return last, (lat - zodiacalLatitudes[last]) / (90 - zodiacalLatitudes[last])
	}

	return bracketAxis(zodiacalLatitudes[:], lat)
}

// ZodiacalColourCorrection returns Leinert et al. (1998) Eq. 22's factor
// f_co: the ratio of the zodiacal light's spectrum to the Sun's, normalised
// to 1 at 500 nm.
//
//	elongation <= 30 deg:  1 + 1.2*log10(lambda/500)  for lambda >= 500 nm
//	                       1 + 0.8*log10(lambda/500)  for lambda <  500 nm
//	elongation >= 90 deg:  1 + 0.9*log10(lambda/500)  for lambda >= 500 nm
//	                       1 + 0.6*log10(lambda/500)  for lambda <  500 nm
//
// linearly interpolated in elongation between 30 and 90 degrees.
//
// The zodiacal spectrum is close to the Sun's but reddened, and reddened more
// strongly at small elongations. Leinert et al. state the sign convention
// outright — f_co below 1 blueward of 500 nm, above 1 redward — which is what
// these coefficients reproduce and what TestZodiacalColourCorrectionSign
// checks.
//
// They give the relation over 220 nm to 2.5 um and caution that f_co "cannot
// be very accurate". Outside that span the nearest end is held rather than
// extrapolated, since the relation is a straight line in log wavelength and
// would eventually cross zero.
func ZodiacalColourCorrection(lambda unit.WavelengthNM, elongation angle.Angle) float64 {
	nm := math.Min(math.Max(float64(lambda), zodiacalColourMinNM), zodiacalColourMaxNM)
	ratio := math.Log10(nm / ZodiacalReferenceNM)

	// Slopes at 30 and at 90 degrees, redward and blueward of 500 nm.
	near, far := 1.2, 0.9
	if nm < ZodiacalReferenceNM {
		near, far = 0.8, 0.6
	}

	deg := foldEclipticLatitude(elongation)
	if elongation.Degrees() > 90 {
		deg = math.Abs(math.Mod(elongation.Degrees(), 360))
	}

	var slope float64

	switch {
	case deg <= 30:
		slope = near
	case deg >= 90:
		slope = far
	default:
		slope = near + (deg-30)/60*(far-near)
	}

	return 1 + slope*ratio
}

// ZodiacalRadiance accumulates the spectral radiance of sunlight scattered by
// interplanetary dust into dst.
//
// The tabulated 500 nm map is carried to other wavelengths and epochs by
// three factors, following Masana et al. (2021) Eq. 18:
//
//   - [ZodiacalColourCorrection], the spectrum's departure from the Sun's;
//   - R^-2.3 for the observer's heliocentric distance (Leinert et al. 1980);
//   - 1 + 0.1*sin(lambda_Earth - 96 deg) above 60 degrees of ecliptic
//     latitude, where the Earth's excursion out of the dust cloud's plane of
//     symmetry matters.
//
// Directions the table excludes return [ErrZodiacalGeometry] rather than a
// number.
func ZodiacalRadiance(dst SpectralRadiance, grid unit.SpectralGrid, geom ZodiacalGeometry) (Flag, error) {
	if len(dst) != grid.Len() {
		return 0, fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
	}

	if geom.SunDistanceAU <= 0 || math.IsNaN(geom.SunDistanceAU) {
		return 0, fmt.Errorf("%w: heliocentric distance %g AU",
			ErrZodiacalGeometry, geom.SunDistanceAU)
	}

	reference, err := ZodiacalBrightnessAt(geom)
	if err != nil {
		return 0, err
	}

	scale := math.Pow(geom.SunDistanceAU, zodiacalHeliocentricExponent) * zodiacalSeasonalFactor(geom)
	elongation := ZodiacalElongation(geom)

	var flags Flag

	for i := range dst {
		lambda := grid.At(i)

		if lambda < zodiacalColourMinNM || lambda > zodiacalColourMaxNM {
			flags |= ExtrapolatedModel
		}

		v := reference * scale * ZodiacalColourCorrection(lambda, elongation)
		if v < 0 {
			flags |= ExtrapolatedModel

			continue
		}

		dst[i] += v
	}

	return flags, nil
}

// ZodiacalElongation is the angular distance from the Sun implied by a
// geometry, which is what the colour correction varies with:
//
//	cos(eps) = cos(dlon) * cos(beta)
func ZodiacalElongation(geom ZodiacalGeometry) angle.Angle {
	lon := angle.Deg(foldDifferentialLongitude(geom.DifferentialLongitude))

	return angle.Acos(math.Min(math.Max(lon.Cos()*geom.EclipticLatitude.Cos(), -1), 1))
}

// zodiacalSeasonalFactor is the plus or minus 10 per cent variation the
// Earth's excursion out of the dust plane produces at high ecliptic latitude.
func zodiacalSeasonalFactor(geom ZodiacalGeometry) float64 {
	if foldEclipticLatitude(geom.EclipticLatitude) < zodiacalSeasonalLatitude {
		return 1
	}

	return 1 + zodiacalSeasonalAmplitude*
		angle.Deg(geom.EarthLongitude.Degrees()-zodiacalNodeLongitude).Sin()
}
