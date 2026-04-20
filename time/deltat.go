package time

import "math"

// DeltaT returns ΔT = TT − UT1 in seconds for a given decimal year.
//
// This implements the Espenak & Meeus (2006) polynomial expressions from the
// "Five Millennium Canon of Solar Eclipses" (NASA/TP-2006-214141), valid for
// years −1999 to +3000.
//
// The base polynomial is from Morrison & Stephenson (2004), which assumes a
// lunar secular acceleration of n-dot = −26.0 arcsec/cy². A correction factor
// is applied to convert to n-dot = −25.858 arcsec/cy² (Chapront et al. 2002,
// from Apollo Lunar Laser Ranging), which is the value used by both the
// ELP-2000/82 ephemeris and JPL DE441:
//
//	c = −0.000012932 × (y − 1955)²
//
// ΔT is the difference between Terrestrial Time (the uniform time scale used
// by planetary ephemerides) and Universal Time (based on Earth's rotation).
// For historical dates before 1972, this is the primary mechanism to convert
// between civil time (UT) and ephemeris time (TT/TDB). For modern dates,
// the relationship is: ΔT = ΔAT + 32.184s − DUT1, where ΔAT comes from
// the NAIF leap-second kernel (LSK) and DUT1 from IERS EOP data.
//
// References:
//   - https://eclipse.gsfc.nasa.gov/LEcat5/deltatpoly.html
//   - Morrison, L. and Stephenson, F. R. (2004). "Historical Values of the
//     Earth's Clock Error ΔT and the Calculation of Eclipses", J. Hist. Astron.,
//     Vol. 35, pp 327–336.
//   - Chapront, Chapront-Touzé, and Francou (2002). Lunar laser ranging value
//     for Moon's secular acceleration: n-dot = −25.858 arcsec/cy².
func DeltaT(year float64) float64 {
	y := year
	var dt float64

	switch {
	case y < -500:
		u := (y - 1820.0) / 100.0
		dt = -20 + 32*u*u

	case y < 500:
		u := y / 100.0
		dt = 10583.6 - 1014.41*u + 33.78311*u*u -
			5.952053*u*u*u - 0.1798452*u*u*u*u +
			0.022174192*u*u*u*u*u + 0.0090316521*u*u*u*u*u*u

	case y < 1600:
		u := (y - 1000.0) / 100.0
		dt = 1574.2 - 556.01*u + 71.23472*u*u +
			0.319781*u*u*u - 0.8503463*u*u*u*u -
			0.005050998*u*u*u*u*u + 0.0083572073*u*u*u*u*u*u

	case y < 1700:
		t := y - 1600
		dt = 120 - 0.9808*t - 0.01532*t*t + t*t*t/7129.0

	case y < 1800:
		t := y - 1700
		dt = 8.83 + 0.1603*t - 0.0059285*t*t +
			0.00013336*t*t*t - t*t*t*t/1174000.0

	case y < 1860:
		t := y - 1800
		dt = 13.72 - 0.332447*t + 0.0068612*t*t +
			0.0041116*t*t*t - 0.00037436*t*t*t*t +
			0.0000121272*t*t*t*t*t - 0.0000001699*t*t*t*t*t*t +
			0.000000000875*t*t*t*t*t*t*t

	case y < 1900:
		t := y - 1860
		dt = 7.62 + 0.5737*t - 0.251754*t*t +
			0.01680668*t*t*t - 0.0004473624*t*t*t*t +
			t*t*t*t*t/233174.0

	case y < 1920:
		t := y - 1900
		dt = -2.79 + 1.494119*t - 0.0598939*t*t +
			0.0061966*t*t*t - 0.000197*t*t*t*t

	case y < 1941:
		t := y - 1920
		dt = 21.20 + 0.84493*t - 0.076100*t*t + 0.0020936*t*t*t

	case y < 1961:
		t := y - 1950
		dt = 29.07 + 0.407*t - t*t/233.0 + t*t*t/2547.0

	case y < 1986:
		t := y - 1975
		dt = 45.45 + 1.067*t - t*t/260.0 - t*t*t/718.0

	case y < 2005:
		t := y - 2000
		dt = 63.86 + 0.3345*t - 0.060374*t*t +
			0.0017275*t*t*t + 0.000651814*t*t*t*t +
			0.00002373599*t*t*t*t*t

	case y < 2050:
		t := y - 2000
		dt = 62.92 + 0.32217*t + 0.005589*t*t

	case y < 2150:
		dt = -20 + 32*((y-1820.0)/100.0)*((y-1820.0)/100.0) -
			0.5628*(2150-y)

	default:
		u := (y - 1820.0) / 100.0
		dt = -20 + 32*u*u
	}

	// Secular acceleration correction: Morrison & Stephenson assume
	// n-dot = -26.0 arcsec/cy², but the LLR value (used by ELP-2000/82
	// and DE441) is -25.858. This adjusts for the difference.
	dt += -0.000012932 * (y - 1955) * (y - 1955)

	return dt
}

// DeltaTUncertainty returns the estimated standard error σ of ΔT in seconds
// for a given year.
//
// The uncertainty arises from fluctuations in Earth's rotation rate that
// are not captured by the smooth polynomial model. Three regimes are used:
//
// For −1000 to +1200 CE: Morrison & Stephenson (2004) parabolic model:
//
//	σ = 0.8 × t² seconds, where t = (year − 1820) / 100
//
// For 1300 to 1600 CE: decade fluctuations give σ ≈ 20 seconds.
//
// For years outside the observed epoch (before −500 CE or after 2005 CE):
// Huber (2000) Brownian motion model with drift:
//
//	σ = 365.25 × N × √(N×Q/3 × (1 + N/M)) / 1000
//	where N = |year − calibrationYear|, M = 2500, Q = 0.058 ms²/yr
//
// For the telescopic era (1600–present), uncertainties decrease from ~5s
// to effectively zero for modern observations.
//
// References:
//   - https://eclipse.gsfc.nasa.gov/LEcat5/uncertainty.html
//   - Morrison, L. and Stephenson, F. R. (2004).
//   - Huber, P. J. (2000). "Modeling the Length of Day and Extrapolating
//     the Rotation of the Earth".
func DeltaTUncertainty(year float64) float64 {
	const (
		M = 2500.0 // observed ΔT measurement span (years)
		Q = 0.058  // intrinsic LOD variability (ms²/yr)
	)

	switch {
	case year < -500:
		// Huber Brownian motion model, calibration year = -500
		N := math.Abs(year - (-500))
		return 365.25 * N * math.Sqrt(N*Q/3.0*(1.0+N/M)) / 1000.0

	case year < 1200:
		// Morrison & Stephenson parabolic model
		t := (year - 1820.0) / 100.0
		return 0.8 * t * t

	case year < 1600:
		// Decade fluctuations dominate
		return 20.0

	case year < 1700:
		return 5.0

	case year < 1800:
		return 2.0

	case year < 1860:
		return 1.0

	case year < 1900:
		return 0.5

	case year < 1955:
		return 0.2

	case year <= 2005:
		// Direct observations — effectively zero uncertainty
		return 0.0

	default:
		// Huber Brownian motion model, calibration year = 2005
		N := math.Abs(year - 2005)
		return 365.25 * N * math.Sqrt(N*Q/3.0*(1.0+N/M)) / 1000.0
	}
}
