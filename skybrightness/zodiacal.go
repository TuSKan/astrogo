package skybrightness

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
)

// ZodiacalLight is the zodiacal-light component, interpolated from the
// Leinert et al. (1998), A&AS 127, 1, Table 17 grid of zodiacal-light spectral
// radiance at 500 nm (in units of 10⁻⁸ W m⁻² sr⁻¹ µm⁻¹) as a function of
// helioecliptic longitude |λ − λ☉| and ecliptic latitude |β|.
type ZodiacalLight struct {
	provider eph.Provider
}

// NewZodiacalLight creates a zodiacal-light component using the given ephemeris
// provider for the Sun's position. A nil provider falls back to
// ephemeris.Default().
func NewZodiacalLight(p eph.Provider) ZodiacalLight {
	if p == nil {
		p = eph.Default()
	}

	return ZodiacalLight{provider: p}
}

// Radiance returns the zodiacal-light radiance toward altaz at the epoch carried
// by ctx.
func (z ZodiacalLight) Radiance(altaz coord.AltAz, ctx *coord.Context) (Nanolambert, error) {
	if ctx == nil {
		return 0, ErrNilContext
	}

	t := ctx.Time()

	targetICRS, err := ctx.AltAzToICRS(altaz)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: alt-az to ICRS: %w", err)
	}

	targetEcl := coord.ICRSToEcliptic(targetICRS, t)

	sunVec, err := eph.Position(z.provider, eph.Sun, t)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: sun position: %w", err)
	}

	sunICRS, err := eph.ToICRS(sunVec)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: sun ICRS: %w", err)
	}

	sunEcl := coord.ICRSToEcliptic(sunICRS, t)

	dLon := helioLongitude(targetEcl.Lon().Degrees(), sunEcl.Lon().Degrees())
	beta := math.Abs(targetEcl.Lat().Degrees())

	si := zodiTable.at(dLon, beta)

	return siToSurfaceBrightnessV(si).Nanolamberts(), nil
}

// helioLongitude returns the helioecliptic longitude |λ − λ☉| folded into
// [0,180]°.
func helioLongitude(lon, sunLon float64) float64 {
	d := math.Mod(math.Abs(lon-sunLon), 360)
	if d > 180 {
		d = 360 - d
	}

	return d
}

// siPerS10 is the Leinert et al. (1998) conversion between S10(V)⊙ units (one
// V=10 solar-type star per square degree) and the SI spectral radiance of
// Table 17: 1 S10(V)⊙ = 1.28 × 10⁻⁸ W m⁻² sr⁻¹ µm⁻¹ at 500 nm.
const siPerS10 = 1.28

// zodiZeroPoint converts a Table 17 cell (SI spectral radiance, in units of
// 10⁻⁸ W m⁻² sr⁻¹ µm⁻¹ at 500 nm) to a V-band surface brightness:
//
//	m_V = zodiZeroPoint − 2.5·log₁₀(B_SI)
//
// The chain is exact and fully sourced: B_SI / 1.28 gives S10(V)⊙ units
// (Leinert et al. 1998), and one S10(V)⊙ is one V=10 star per square degree,
// so spread over a square degree (3600² arcsec²) the per-arcsec² zero point is
// 10 + 2.5·log₁₀(3600²) + 2.5·log₁₀(1.28). As a cross-check this maps the
// ecliptic pole (77 in Table-17 units = 60 S10) to ~23.3 V mag/arcsec²,
// matching the known dark-sky zodiacal minimum.
var zodiZeroPoint = 10 + 2.5*math.Log10(3600.0*3600.0) + 2.5*math.Log10(siPerS10)

// siToSurfaceBrightnessV converts a Table 17 cell value (SI spectral radiance,
// units of 10⁻⁸ W m⁻² sr⁻¹ µm⁻¹) to a V-band surface brightness (mag/arcsec²).
func siToSurfaceBrightnessV(si float64) SurfaceBrightnessV {
	return SurfaceBrightnessV(zodiZeroPoint - 2.5*math.Log10(si))
}

// zodiLon are the helioecliptic-longitude breakpoints (degrees) of zodiTable.
var zodiLon = []float64{0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 60, 75, 90, 105, 120, 135, 150, 165, 180}

// zodiLat are the ecliptic-latitude breakpoints (degrees) of zodiTable.
var zodiLat = []float64{0, 5, 10, 15, 20, 25, 30, 45, 60, 75, 90}

// zodiValues holds Leinert et al. (1998), A&AS 127, 1, Table 17: zodiacal-light
// spectral radiance at 500 nm in units of 10⁻⁸ W m⁻² sr⁻¹ µm⁻¹, indexed
// [longitudeIndex][latitudeIndex] over (zodiLon, zodiLat).
//
// Transcribed from the primary source (Leinert et al. 1998 §8) and
// cross-validated against the Table 16 S10(V)⊙ values via the 1.28×10⁻⁸ W
// conversion (see siPerS10): pole 77↔60 S10, gegenschein (180°,0°) 230↔180,
// (90°,0°) 259↔202, (60°,0°) 505↔395, (30°,0°) 2480↔1940 — all consistent to
// the 1.28 factor. The four near-Sun high-latitude cells blank in the original
// (solar-avoidance region, not observable at night) are filled from the nearest
// covered longitude.
var zodiValues = [][]float64{
	//  β=0    5     10    15    20    25    30    45    60    75    90
	{3140, 1610, 985, 640, 275, 150, 100, 251, 146, 100, 77},     // λ=0
	{2940, 1540, 945, 625, 271, 150, 100, 251, 146, 100, 77},     // λ=5
	{4740, 2470, 1370, 865, 590, 264, 148, 100, 146, 100, 77},    // λ=10
	{11500, 6780, 3440, 1860, 1110, 755, 525, 251, 146, 100, 77}, // λ=15
	{6400, 4480, 2410, 1410, 910, 635, 454, 237, 141, 99, 77},    // λ=20
	{3840, 2830, 1730, 1100, 749, 545, 410, 223, 136, 97, 77},    // λ=25
	{2480, 1870, 1220, 845, 615, 467, 365, 207, 131, 95, 77},     // λ=30
	{1650, 1270, 910, 680, 510, 397, 320, 193, 125, 93, 77},      // λ=35
	{1180, 940, 700, 530, 416, 338, 282, 179, 120, 92, 77},       // λ=40
	{910, 730, 555, 442, 356, 292, 250, 166, 116, 90, 77},        // λ=45
	{505, 442, 352, 292, 243, 209, 183, 134, 104, 86, 77},        // λ=60
	{338, 317, 269, 227, 196, 172, 151, 116, 93, 82, 77},         // λ=75
	{259, 251, 225, 193, 166, 147, 132, 104, 86, 79, 77},         // λ=90
	{212, 210, 197, 170, 150, 133, 119, 96, 82, 77, 77},          // λ=105
	{188, 186, 177, 154, 138, 125, 113, 90, 77, 74, 77},          // λ=120
	{179, 178, 166, 147, 134, 122, 110, 90, 77, 73, 77},          // λ=135
	{179, 178, 165, 148, 137, 127, 116, 96, 79, 72, 77},          // λ=150
	{196, 192, 179, 165, 151, 141, 131, 104, 82, 72, 77},         // λ=165
	{230, 212, 195, 178, 163, 148, 134, 105, 83, 72, 77},         // λ=180
}

// zodiTable is the bilinear-interpolation view of the Leinert Table 17 grid.
var zodiTable = grid2D{xs: zodiLon, ys: zodiLat, v: zodiValues}
