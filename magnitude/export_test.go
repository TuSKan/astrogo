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
