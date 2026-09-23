package coord_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// Two clouds with the same Galactic velocity, on opposite sides of the solar
// apex, have barycentric velocities that differ by twice the solar motion.
// Referring both to the Local Standard of Rest removes the Sun's own motion and
// leaves the physics.
func ExampleLSRCorrection() {
	apex, speed := coord.LSRApex(coord.LSRDynamical)

	toward := apex
	away := coord.NewICRS(apex.RA().Add(angle.Deg(180)).Wrap360(), apex.Dec().MulScalar(-1))

	// Both measured at rest against the barycenter.
	const rvBarycentric = 0.0

	fmt.Printf("solar motion:       %.2f km/s\n", speed.KmPerSec())
	fmt.Printf("toward the apex:    %+.2f km/s\n",
		rvBarycentric+coord.LSRCorrection(toward, coord.LSRDynamical).KmPerSec())
	fmt.Printf("away from it:       %+.2f km/s\n",
		rvBarycentric+coord.LSRCorrection(away, coord.LSRDynamical).KmPerSec())

	// Output:
	// solar motion:       18.04 km/s
	// toward the apex:    +18.04 km/s
	// away from it:       -18.04 km/s
}

// The convention is named at the call site because the published values
// disagree, and by more than any radial velocity worth correcting.
func ExampleLSRCorrection_conventions() {
	target := coord.NewICRS(angle.Deg(266.4), angle.Deg(-29.0)) // the Galactic center

	for _, kind := range []coord.LSRKind{coord.LSRDynamical, coord.LSRDelhaye} {
		fmt.Printf("%-22s %+.3f km/s\n", kind, coord.LSRCorrection(target, kind).KmPerSec())
	}

	// This direction is the one where the answer can be read straight off the
	// papers: the Galactic center is the +U axis, so each correction is that
	// convention's U component — 11.1 and 9 km/s — short by the tenth of a
	// degree between the round numbers above and the true Galactic center.

	// Output:
	// LSR (Schönrich+ 2010)  +11.084 km/s
	// LSRD (Delhaye 1965)    +8.985 km/s
}
