package starlight

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/magnitude"
)

// ErrBrightStar is returned when a bright star cannot be placed.
var ErrBrightStar = errors.New("starlight: unusable bright star")

// BrightStar is a star bright enough that Gaia does not record it.
//
// Gaia saturates on the brightest sky: measured against DR3, 73 of the 4,992
// Hipparcos stars brighter than V = 6 have no counterpart within five
// arcseconds, and they are exactly the ones anybody could name — Sirius,
// Canopus, Arcturus, Alpha Centauri, Vega, Capella, Rigel, Procyon, Achernar,
// Betelgeuse. A map built from Gaia alone is missing them.
type BrightStar struct {
	// HIP is the Hipparcos identifier, carried for provenance.
	HIP int

	// RA and Dec are ICRS coordinates.
	RA, Dec angle.Angle

	// Vmag is the Johnson V magnitude, which the Hipparcos catalogue supplies
	// directly.
	Vmag float64

	// Mag holds the star's magnitude in every band it is known in, keyed by
	// the band name the map uses. V is always present and equals Vmag.
	//
	// # Where the other bands come from, and why none of them is a fit
	//
	// Unlike the Gaia path there is no colour polynomial here. Every band is
	// arithmetic on published colour indices:
	//
	//	B = V + (B-V)          Hipparcos I/239/hip_main
	//	I = V - (V-I)          Hipparcos I/239/hip_main
	//	R = V - (V-I) + (R-I)  the last from the Bright Star Catalogue V/50
	//
	// The R identity is the one worth stating: V-R = (V-I) - (R-I), so R
	// follows exactly from two catalogued indices with nothing interpolated.
	// Hipparcos publishes no R and no V-R, and the alternative — a
	// colour-colour relation predicting V-R from B-V — would be a fit, which
	// is what this package refuses everywhere else. A star that the Bright
	// Star Catalogue does not cover therefore has no R entry at all rather
	// than an estimated one.
	Mag map[string]float64

	// vMinusI is the Hipparcos colour, kept only long enough to turn the
	// Bright Star Catalogue's R-I into an R magnitude. Unexported because it
	// is a step in building Mag rather than part of what a star is.
	vMinusI    float64
	hasVminusI bool

	// pmRA and pmDec are the proper motion in milliarcseconds a year, carried
	// so a position at the Hipparcos epoch can be brought to the epoch of
	// whatever it is being matched against.
	pmRA, pmDec float64
}

// magnitudeIn returns the star's magnitude in a band, and whether it has one.
func (s BrightStar) magnitudeIn(band string) (float64, bool) {
	if s.Mag != nil {
		v, ok := s.Mag[band]

		return v, ok
	}

	// A star carrying only Vmag still answers for V, which keeps a
	// hand-constructed one usable.
	if band == "V" {
		return s.Vmag, true
	}

	return 0, false
}

// AddBrightStars sums stars into an existing map's band.
//
// # Why this is addition and not a rebuild
//
// Radiance is linear, so a star's contribution adds to whatever the pixel
// already holds. The Gaia aggregation does not have to be repeated to include
// them — which matters, because that aggregation is 787 queries against a
// shared service and this is a few dozen rows.
//
// # What it corrects
//
// Measured against the order-8 Gaia DR3 map, the missing stars carry about
// 2.6 per cent of the all-sky mean radiance. Masana et al. (2021) put the
// equivalent correction at around 20 per cent, but that is a **DR2** figure:
// DR2 lacked a counterpart for some 35,000 Hipparcos stars, DR3 recovered
// nearly all of them, and what remains is only the handful Gaia saturates on.
// The correction shrank because the catalogue improved, not because the
// physics changed.
//
// A star outside the map's grid, or with a magnitude that cannot produce a
// finite irradiance, is an error rather than a silently dropped row: these are
// the brightest objects in the sky and losing one is not a rounding error.
func AddBrightStars(m *Map, name string, band magnitude.Passband, stars []BrightStar) error {
	if m == nil {
		return fmt.Errorf("%w: no map", ErrBrightStar)
	}

	values, ok := m.bands[name]
	if !ok {
		return fmt.Errorf("%w: %q", ErrBand, name)
	}

	// The zero point comes from the passband's own published calibration, the
	// same one the Gaia band conversion uses, so the two paths into a map
	// cannot rest on different numbers.
	zeroFlux, err := VegaZeroFlux(band)
	if err != nil {
		return fmt.Errorf("%w: band %q: %w", ErrBrightStar, name, err)
	}

	grid := m.Grid()
	solidAngle := 4 * math.Pi / float64(grid.NumPixels())

	var added int

	for _, s := range stars {
		mag, ok := s.magnitudeIn(name)
		if !ok {
			// Not every catalogue covers every band, and a star with no
			// magnitude in this one contributes nothing to it. Silently, and
			// deliberately: the alternative is to invent a colour, and the
			// count is reported below so the gap is visible rather than
			// assumed away.
			continue
		}

		if math.IsNaN(mag) || math.IsInf(mag, 0) {
			return fmt.Errorf("%w: HIP %d has %s = %v", ErrBrightStar, s.HIP, name, mag)
		}

		pixel := grid.PixelOf(s.RA, s.Dec)
		if pixel < 0 || pixel >= int64(len(values)) {
			return fmt.Errorf("%w: HIP %d falls on pixel %d, outside [0, %d)",
				ErrBrightStar, s.HIP, pixel, len(values))
		}

		values[pixel] += zeroFlux * math.Pow(10, -0.4*mag) / solidAngle
		added++
	}

	// Record the addition on the map. A published map holding Hipparcos
	// photometry while claiming only gaiadr3.gaia_source misdescribes itself,
	// and the difference is 6.4 per cent of the sky.
	if added > 0 {
		note := fmt.Sprintf("%d of %d Hipparcos stars with no Gaia DR3 counterpart added in %s, "+
			"from I/239/hip_main photometry", added, len(stars), name)
		if m.Source == "" {
			m.Source = note
		} else {
			m.Source += "; " + note
		}
	}

	return nil
}

// BrightStarsMissingFromGaia reports which of the given Hipparcos stars have no
// Gaia counterpart, propagating Hipparcos positions to Gaia's epoch first.
//
// Hipparcos is catalogued at J1991.25 and Gaia DR3 at J2016.0, so a star has
// moved by nearly twenty-five years of proper motion between them. Matching
// without that shift misses genuine counterparts for exactly the nearby, fast
// stars that are also the brightest, which would then be added twice.
//
// radius is the match tolerance. The result is not sensitive to it: at five
// arcseconds 73 stars are unmatched and at fifteen, 69 — a five per cent change
// in a correction that is itself under three per cent of the sky.
func BrightStarsMissingFromGaia(
	hipparcos []BrightStar,
	pmRA, pmDec []float64,
	gaiaRA, gaiaDec []angle.Angle,
	radius angle.Angle,
) ([]BrightStar, error) {
	if len(pmRA) != len(hipparcos) || len(pmDec) != len(hipparcos) {
		return nil, fmt.Errorf("%w: %d stars but %d/%d proper motions",
			ErrBrightStar, len(hipparcos), len(pmRA), len(pmDec))
	}

	if len(gaiaRA) != len(gaiaDec) {
		return nil, fmt.Errorf("%w: %d Gaia positions but %d declinations",
			ErrBrightStar, len(gaiaRA), len(gaiaDec))
	}

	const epochGap = 2016.0 - 1991.25 // Gaia DR3 minus Hipparcos, in years

	missing := make([]BrightStar, 0, 128)

	for i, s := range hipparcos {
		ra, dec := propagate(s.RA, s.Dec, pmRA[i], pmDec[i], epochGap)

		if !hasCounterpart(ra, dec, gaiaRA, gaiaDec, radius) {
			missing = append(missing, s)
		}
	}

	return missing, nil
}

// propagate moves a position by proper motion, in milliarcseconds per year,
// over the given number of years.
func propagate(ra, dec angle.Angle, pmRA, pmDec, years float64) (angle.Angle, angle.Angle) {
	// Proper motion in right ascension is already the great-circle rate for
	// these catalogues, so it is divided by cos(dec) to become a coordinate
	// increment rather than multiplied by it.
	cosDec := math.Cos(dec.Radians())
	if cosDec == 0 {
		cosDec = 1e-9
	}

	const masToDeg = 1.0 / 3.6e6

	return ra + angle.Deg(pmRA*masToDeg*years/cosDec), dec + angle.Deg(pmDec*masToDeg*years)
}

// hasCounterpart reports whether any Gaia position lies within radius.
func hasCounterpart(ra, dec angle.Angle, gaiaRA, gaiaDec []angle.Angle, radius angle.Angle) bool {
	cosDec := math.Cos(dec.Radians())

	for i := range gaiaRA {
		dDec := (gaiaDec[i] - dec).Degrees()
		if math.Abs(dDec) > radius.Degrees() {
			continue // cheap reject before the trigonometry
		}

		dRA := (gaiaRA[i] - ra).Degrees() * cosDec
		if math.Hypot(dRA, dDec) <= radius.Degrees() {
			return true
		}
	}

	return false
}

// pixelOfStar is used by the tests to check placement without exporting the
// grid arithmetic.
func pixelOfStar(grid coord.HEALPix, s BrightStar) int64 {
	return grid.PixelOf(s.RA, s.Dec)
}
