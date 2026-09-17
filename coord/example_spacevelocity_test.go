package coord_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// Proper motion is an angular rate and radial velocity is a linear one, so
// neither can be compared with the other. A space velocity puts all three
// components in one unit.
func ExampleSpaceVelocity() {
	// Barnard's Star: the largest proper motion known, and close enough that
	// its distance is not in doubt.
	star := coord.NewICRSWithKinematics(
		angle.Deg(269.452), angle.Deg(4.693),
		angle.Arcsec(-0.79847), angle.Arcsec(10.33777),
		angle.Arcsec(0.54698), -110.6,
	)

	v, ok := coord.SpaceVelocity(star)
	if !ok {
		fmt.Println("no velocity available")
		return
	}

	// Split it the way an observation does: along the line of sight, and
	// across it.
	unit := star.ToUnitVector()
	radial := v.Dot(unit)
	transverse := v.Sub(unit.MulScalar(radial)).Norm()

	fmt.Printf("radial:     %7.1f km/s\n", radial)
	fmt.Printf("transverse: %7.1f km/s\n", transverse)
	fmt.Printf("total:      %7.1f km/s\n", v.Norm())

	// The transverse component is the 10.34 arcsec a year, read at 1.83 pc.
	// Neither number is larger than the other in any meaningful sense until
	// both are in km/s, which is the point.

	// Output:
	// radial:      -110.6 km/s
	// transverse:    89.8 km/s
	// total:        142.5 km/s
}

// A space velocity in km/s needs a distance, and says so when it has none.
func ExampleSpaceVelocity_withoutAParallax() {
	// A proper-motion catalogue with no parallax column, which describes most
	// of the pre-Hipparcos ones.
	star := coord.NewICRSWithKinematics(
		angle.Deg(123.4), angle.Deg(-35.6),
		angle.Arcsec(0.150), angle.Arcsec(0.220), 0, -22.4,
	)

	if _, ok := coord.SpaceVelocity(star); !ok {
		fmt.Println("no space velocity: the same 150 mas/yr is 7 km/s at 10 pc and 700 at 1 kpc")
	}

	// A frame conversion is a different matter. It rotates the proper motion,
	// and the distance divides out of that, so the same star converts perfectly
	// well — which is what #331 fixed.
	fk5 := coord.ICRSToFK5(star, coord.J2000Epoch)
	pmRA, pmDec, _ := fk5.ProperMotion()

	fmt.Printf("FK5 proper motion: %.1f, %.1f mas/yr\n",
		pmRA.Arcseconds()*1000, pmDec.Arcseconds()*1000)

	// The motion comes out changed rather than copied, by about 1 mas/yr: FK5
	// spins slowly against the ICRS, so a star's apparent motion genuinely
	// differs between the two frames. That is the conversion working, and it
	// needed no distance at all.

	// Output:
	// no space velocity: the same 150 mas/yr is 7 km/s at 10 pc and 700 at 1 kpc
	// FK5 proper motion: 151.0, 220.1 mas/yr
}
