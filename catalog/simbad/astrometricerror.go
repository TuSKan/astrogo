package simbad

import (
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/internal/finite"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/time"
)

// gaiaCoordinateEpochs are the epochs of the Gaia releases SIMBAD takes
// positions from, keyed by the coo_bibcode it cites for them. SIMBAD moves
// such a position to J2000.0 and keeps Gaia's errors as Gaia publishes them,
// at Gaia's epoch: Barnard's star carries Gaia's 0.026 mas, where sixteen
// years of its proper-motion error make 0.5 mas by J2000.0. SIMBAD's errors
// for a position from any other source hold at J2000.0. readKinematics
// reports a position from one of these releases at its epoch, where the
// errors hold.
var gaiaCoordinateEpochs = map[string]time.Time{
	"2020yCat.1350....0G": time.FromJD(2457389.0, time.TT),   // Gaia EDR3, J2016.0
	"2018yCat.1345....0G": time.FromJD(2457206.375, time.TT), // Gaia DR2, J2015.5
}

// astrometricError reads a row's error ellipses, the position's in mas and
// the proper motion's in mas/yr, each with its position angle in degrees.
// An ellipse the row does not have both axes of is left zero, which
// AstrometricError reads as unpublished.
func astrometricError(row []string, colIdx map[string]int) resolve.AstrometricError {
	cell := func(name string) (string, bool) {
		i, ok := colIdx[name]
		if !ok || i >= len(row) {
			return "", false
		}

		return row[i], true
	}

	number := func(name string) float64 {
		s, ok := cell(name)
		if !ok || s == "" {
			return 0
		}

		v, _ := finite.Parse(s)

		return v
	}

	var e resolve.AstrometricError

	if maj, minor := number("coo_err_maj"), number("coo_err_min"); maj > 0 && minor > 0 {
		e.RA, e.Dec, e.RADec = errorEllipse(maj, minor, number("coo_err_angle"))

		if bib, ok := cell("coo_bibcode"); ok {
			e.Epoch = gaiaCoordinateEpochs[bib]
		}
	}

	if maj, minor := number("pm_err_maj"), number("pm_err_min"); maj > 0 && minor > 0 {
		e.PmRA, e.PmDec, e.PmRAPmDec = errorEllipse(maj, minor, number("pm_err_angle"))
	}

	return e
}

// errorEllipse is the standard errors of the α* and δ components of an
// error ellipse, and their correlation: major and minor are the standard
// errors along its axes in mas (or mas/yr), and positionAngle is the major
// axis's direction in degrees east of north.
func errorEllipse(major, minor, positionAngle float64) (ra, dec angle.Angle, corr float64) {
	s, c := math.Sincos(positionAngle * math.Pi / 180)

	// The major axis points along (sin θ, cos θ) in (α*, δ), the minor
	// along (cos θ, −sin θ).
	varRA := major*major*s*s + minor*minor*c*c
	varDec := major*major*c*c + minor*minor*s*s
	cov := (major*major - minor*minor) * s * c

	return angle.Arcsec(math.Sqrt(varRA) / 1000), angle.Arcsec(math.Sqrt(varDec) / 1000), cov / math.Sqrt(varRA*varDec)
}
