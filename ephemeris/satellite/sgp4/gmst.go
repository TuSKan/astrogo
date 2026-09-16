package sgp4

import "math"

// twoPi is spelled out rather than taken from a shared constant because every
// modulo in this package is against it and the reference writes it inline.
const twoPi = 2.0 * math.Pi

// gmst82 is Greenwich mean sidereal time in radians, by the 1982 IAU
// expression evaluated on UT1, which SGP4 supplies as UTC.
//
// # Why this is not time.Time.GAST, and must not become it
//
// astrogo has a proper sidereal time: [github.com/TuSKan/astrogo/time]'s GAST
// is IAU 2006/2000A with real Earth orientation parameters, and it is a
// materially better answer to the question "what is the sidereal time". Using
// it here would make this package *worse*.
//
// SGP4's lunisolar terms were fitted against this expression. The model is
// internally consistent rather than accurate: its sidereal time is one of its
// own definitions, and replacing it with a better one leaves every coefficient
// that was tuned against it describing something slightly different. The
// element sets being propagated were fitted through code containing exactly
// these coefficients.
//
// So: the model keeps its own GMST, and TEME to GCRS — where accuracy genuinely
// is the question — uses astrogo's. The boundary is the point.
//
// Vallado 2004, page 191, equation 3-45.
func gmst82(jdut1 float64) float64 {
	tut1 := (jdut1 - 2451545.0) / 36525.0

	// Seconds of time. The association is the reference's, term for term.
	sec := -6.2e-6*tut1*tut1*tut1 +
		0.093104*tut1*tut1 +
		(876600.0*3600+8640184.812866)*tut1 +
		67310.54841

	// 360/86400 = 1/240, so this is seconds to degrees to radians.
	rad := math.Mod(sec*(math.Pi/180.0)/240.0, twoPi)
	if rad < 0.0 {
		rad += twoPi
	}

	return rad
}

// gstoAFSPC is the sidereal time [ModeAFSPC] uses instead of [gmst82].
//
// It counts whole days from 0 January 1970 and advances a fixed rate from a
// fixed 1970 value, which is what the original Space Command implementation
// did. It is not an approximation of gmst82 — it is a different definition,
// and reproducing output generated in AFSPC mode means reproducing it.
//
// epoch1950 is days from 0 January 1950, 0h, which is the argument SGP4 carries
// its epoch in.
func gstoAFSPC(epoch1950 float64) float64 {
	const (
		// Radians of Earth rotation per day, less one revolution — the amount
		// by which a solar day exceeds a sidereal one, 2*pi*(86400/86164.09) -
		// 2*pi. Verified to 1.4e-7 relative.
		//
		// Close to 2*pi/365.25 and NOT equal to it: the two differ in the fifth
		// digit, and they are close because the excess IS the Earth's orbital
		// motion over a day.
		c1 = 1.72027916940703639e-2

		// Greenwich mean sidereal time at the origin this expression counts
		// from, 1970 January 0.0 — that is, 1969-12-31 0h UT, JD 2440586.5.
		//
		// Not an approximation of one: gmst82 evaluated at that Julian Date
		// returns this number to ten digits, which means the two modes agree
		// exactly at the origin and diverge only as they accumulate from it.
		// TestAFSPCOriginIsTheSameSiderealTime asserts it, which is what ties
		// the two paths together instead of leaving them two unrelated
		// expressions that happen to sit in the same file.
		thgr70 = 1.7321343856509374

		// A small secular correction, quadratic in days since the origin. It is
		// named fk5r in the reference and carries no comment there; over the 56
		// years from 1970 to 2026 it contributes 2.1e-6 rad, which is 0.44
		// arcseconds. What exactly it corrects is not something this code can
		// establish, so it is not claimed here.
		fk5r = 5.07551419432269442e-15
	)

	ts70 := epoch1950 - 7305.0

	// The 1e-8 nudge is Vallado's: it keeps a day boundary that lands exactly
	// on an integer from falling into the previous day through rounding.
	ds70 := math.Floor(ts70 + 1.0e-8)
	tfrac := ts70 - ds70

	gsto := math.Mod(thgr70+c1*ds70+(c1+twoPi)*tfrac+ts70*ts70*fk5r, twoPi)
	if gsto < 0.0 {
		gsto += twoPi
	}

	return gsto
}
