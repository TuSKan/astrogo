package skybrightness

import "github.com/TuSKan/astrogo/coord"

// SeparationForTest exposes the scattering-angle helper.
//
// Unexported in the package because it is an implementation detail of the
// scattering integral rather than a geometry utility callers should reach for
// — coord.Separation is that. Exposed here because its conditioning near zero
// is worth testing directly, and testing it through the whole integral would
// bury the thing being measured.
func SeparationForTest(a, b coord.AltAz) float64 { return separation(a, b) }
