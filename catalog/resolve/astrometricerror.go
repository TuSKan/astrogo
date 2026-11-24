package resolve

import (
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/epoch"
)

// AstrometricError is the published uncertainty of a position and its proper
// motion: the standard errors of the declination, of α* = α·cos δ, and of
// μα* and μδ, and the correlations between them, as Gaia publishes them.
//
// A zero RA or Dec is a position error the provider does not publish, and a
// zero PmRA or PmDec a motion error it does not publish. A zero correlation
// is an uncorrelated pair, or one the provider does not publish.
type AstrometricError struct {
	// RA and Dec are the standard errors of α* and δ.
	RA, Dec angle.Angle
	// PmRA and PmDec are the standard errors of μα* and μδ, per Julian
	// year.
	PmRA, PmDec angle.Angle
	// RADec, RAPmRA, RAPmDec, DecPmRA, DecPmDec and PmRAPmDec are the
	// correlation coefficients of each pair, Gaia's ra_dec_corr through
	// pmra_pmdec_corr.
	RADec, RAPmRA, RAPmDec, DecPmRA, DecPmDec, PmRAPmDec float64
	// Epoch is the instant the errors hold at. Zero means the Target's own
	// Epoch, which is where a provider's errors hold unless it states
	// another: a catalog that moves a position and not its errors, as SIMBAD
	// does a position it takes from Gaia, states the epoch the errors still
	// hold at.
	Epoch epoch.Time
}
