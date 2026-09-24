package magnitude

// The Mars model's internals, for tests that check each stage against the
// published values for that stage rather than only the final magnitude.
var (
	MarsMag                      = marsMag
	MarsCorrection               = marsCorrection
	MarsRotationCorrection       = &marsRotationCorrection
	MarsOrbitalCorrection        = &marsOrbitalCorrection
	MarsEffectiveCentralMeridian = marsEffectiveCentralMeridian
	MarsSolarLongitude           = marsSolarLongitude
	MarsSubLongitudes            = marsSubLongitudes
	EclipticLongitude            = eclipticLongitude
)

// The Pluto–Charon model's internals, for the same reason: the paper
// tabulates its photometry at each visit's phase angle and longitude, not at
// a date, so the model is tested there.
var (
	PlutoCharonMagnitudesAt = plutoCharonMagnitudes
	PlutoCharonGeometry     = plutoCharonGeometry
	AddMagnitudes           = addMagnitudes
	PlutoPhaseCurve         = plutoPhaseCurve.at
	CharonPhaseCurve        = charonPhaseCurve.at
)
