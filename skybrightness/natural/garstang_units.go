package natural

import "math"

// Garstang nanolambert<->V-magnitude conversion constants, carried
// verbatim from astrogo v1's skybrightness/units.go. Unexported: only
// ConstantAirglow/VBandMoonlight may use them — this is
// explicitly NOT public API (docs/skybrightness.md §3's
// photometric-constants table).
//
// nlGarstangExp's coefficient 0.92104 is v1's own historically-published
// value, itself a 5-decimal rounding of Pogson's ratio in natural-log
// form, 0.4*ln(10) = 0.9210340371976184. Because garstangVegaZeroPoint
// below is defined as exactly nlGarstangScale*exp(nlGarstangExp) — the
// same literal 0.92104, not the more precise 0.4*ln(10) — the round trip
// -2.5*log10(garstangNanolambert(v)/garstangVegaZeroPoint) reproduces v to
// the precision of that shared, rounded literal (~1.5e-4 mag at V~22, the
// worst case in this package's test range), not to full float64
// precision. This is carried forward faithfully from v1's own rounding,
// not introduced here — see natural_test.go's
// TestConstantAirglow_RoundTripToHistoricalPrecision for the measured
// bound.
const (
	nlGarstangScale = 34.08
	nlGarstangExp   = 20.7233
)

// garstangNanolambert converts a V mag/arcsec^2 surface brightness to
// linear brightness in nanolamberts (Garstang's convention, as used by
// Krisciunas & Schaefer 1991 and Schaefer 1990).
func garstangNanolambert(v float64) float64 {
	return nlGarstangScale * math.Exp(nlGarstangExp-0.92104*v)
}

// garstangVegaZeroPoint is the nanolambert value at V=0 mag/arcsec^2:
// nlGarstangScale * exp(nlGarstangExp). Used as TopHatJohnsonV's
// VegaZeroPoint.MeanFlambda so a flat "spectrum" of value
// garstangNanolambert(v) integrates back to exactly v through
// VegaSurfaceBrightness.
var garstangVegaZeroPoint = nlGarstangScale * math.Exp(nlGarstangExp)

// topHatVLo, topHatVHi bound the top-hat V-band stand-in passband,
// approximating the Johnson V bandpass (Bessell 1990, PASP 102, 1181,
// effective wavelength ~551 nm) as a flat top hat rather than reproducing
// its real tabulated response curve — a documented Phase 1 simplification
// for a component whose whole purpose is speed, not spectral fidelity.
const (
	topHatVLo = 470.0
	topHatVHi = 700.0
)
