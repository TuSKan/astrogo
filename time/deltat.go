package time

import "math"

// DeltaT returns ΔT = TT − UT1 at t, in seconds: the one value of it every
// conversion in this package uses.
//
// It has three regimes, by where t falls:
//
//   - Before 1960, before UTC existed: the Espenak & Meeus (2006) model, read
//     at t's decimal year. [Time.TT] reads a UTC label from before 1960 as UT
//     through the same function.
//   - From 1960, where the IERS bulletin covers t: the measured value,
//     ΔAT + 32.184 s − DUT1, exactly the difference [Time.TT] and [Time.UT1]
//     put between the labels of one instant.
//   - Past the bulletin's end: ΔT held at its value on the bulletin's last
//     day. See [Time.EOP] for why it is held rather than extrapolated, and
//     [DeltaTUncertainty] for what holding it costs.
//
// With no bulletin loaded, the second and third regimes read DUT1 as zero, as
// the conversions do, and the one-time EOP warning says so.
//
// It used to be the model at every epoch, while the conversions used the
// bulletin, and the two disagreed for one instant: by 6.2 s in 2026, where
// the model, fitted before the Earth's rotation sped up, still rises at
// 0.6 s a year and the measured ΔT has been flat since 2020; and by 134 s in
// 2100, where the conversions held DUT1 at zero (#696).
//
// From 1960 the value comes from the EOP lookup, so it can trigger the same
// lazy load [Time.EOP] does.
func DeltaT(t Time) float64 {
	u := t.UTC()
	if u.jd1+u.jd2 < jd1960 {
		return deltaTModel(u.DecimalYear())
	}

	tt1, tt2 := u.TT().JDParts()
	ut1 := u.UT1Using(dut1OrFallback(u.jd1, u.jd2))

	return ((tt1 - ut1.jd1) + (tt2 - ut1.jd2)) * daySeconds
}

// deltaTModel is ΔT = TT − UT1 in seconds for a decimal year, by the Espenak &
// Meeus (2006) model: what [DeltaT] returns before 1960, and the reading of a
// pre-1960 UTC label as UT in [Time.TT] and [Time.UTC].
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
// Its segments after 1960 are kept, and still tested, as the model; nothing
// reads them for an epoch. From 1960 [DeltaT] is the measured value.
//
// References:
//   - https://eclipse.gsfc.nasa.gov/LEcat5/deltatpoly.html
//   - Morrison, L. and Stephenson, F. R. (2004). "Historical Values of the
//     Earth's Clock Error ΔT and the Calculation of Eclipses", J. Hist. Astron.,
//     Vol. 35, pp 327–336.
//   - Chapront, Chapront-Touzé, and Francou (2002). Lunar laser ranging value
//     for Moon's secular acceleration: n-dot = −25.858 arcsec/cy².
func deltaTModel(year float64) float64 {
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

// DeltaTUncertainty returns the estimated standard error σ of [DeltaT] at t,
// in seconds.
//
// Before 1955 it is the historical model's, from the fluctuations in the
// Earth's rotation that the smooth polynomial does not capture:
//
// For −1000 to +1200 CE: Morrison & Stephenson (2004) parabolic model:
//
//	σ = 0.8 × t² seconds, where t = (year − 1820) / 100
//
// For 1300 to 1600 CE: decade fluctuations give σ ≈ 20 seconds. For the
// telescopic era, uncertainties decrease from ~5 s to 0.2 s by 1955. Before
// −500 CE: the Huber (2000) random walk below, from −500.
//
// From 1955 to the end of the measured record ΔT is observed, and σ is zero.
// The record ends on the IERS bulletin's last day. Past it [DeltaT] holds the
// last measured value, and σ is the Huber (2000) Brownian-motion model of
// the Earth's rotation, from the record's end:
//
//	σ = 365.25 × N × √(N×Q/3 × (1 + N/M)) / 1000
//	where N = years past the end, M = 2500, Q = 0.058 ms²/yr
//
// With no bulletin loaded, the record is taken to end in 2005, the end of the
// observations the model was fitted to, which is where this used to start
// every caller's random walk (#696).
//
// References:
//   - https://eclipse.gsfc.nasa.gov/LEcat5/uncertainty.html
//   - Morrison, L. and Stephenson, F. R. (2004).
//   - Huber, P. J. (2000). "Modeling the Length of Day and Extrapolating
//     the Rotation of the Earth".
func DeltaTUncertainty(t Time) float64 {
	year := t.UTC().DecimalYear()
	if year < 1955 {
		return deltaTUncertaintyHistorical(year)
	}

	// The lookup is what loads a bulletin, if one is to be loaded; Coverage
	// only reports the model already registered.
	mjd := t.MJD()
	_, _, _ = lookupEOP(mjd)

	if _, last, ok := Coverage(); ok {
		return huberSigma((mjd - last) / 365.25)
	}

	return huberSigma(year - 2005)
}

// huberSigma is the Huber (2000) random walk's σ of ΔT, in seconds, n years
// from the last observation; zero at or before it.
func huberSigma(n float64) float64 {
	const (
		M = 2500.0 // observed ΔT measurement span (years)
		Q = 0.058  // intrinsic LOD variability (ms²/yr)
	)

	if n <= 0 {
		return 0
	}

	return 365.25 * n * math.Sqrt(n*Q/3.0*(1.0+n/M)) / 1000.0
}

// deltaTUncertaintyHistorical is [DeltaTUncertainty] before 1955, by decimal
// year.
func deltaTUncertaintyHistorical(year float64) float64 {
	switch {
	case year < -500:
		// Huber Brownian motion model, calibration year = -500
		return huberSigma(-500 - year)

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

	default:
		return 0.2
	}
}
