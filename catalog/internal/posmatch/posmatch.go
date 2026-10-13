// Package posmatch holds the one positional rule that catalog's Resolver and
// catalog/xmatch's Match share: where a Target is once its own motion is
// attached, and how far apart two such positions are at a common epoch.
//
// It is one package because the rule used to be written twice. #627 found
// that neither copy ever applied a proper motion; #629 fixed the Resolver's
// copy, and catalog/xmatch kept the bug until #726.
package posmatch

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// Coord returns t's position carrying t's own proper motion, parallax and
// radial velocity.
//
// Every provider keeps those in the Target's fields and builds Coord from RA
// and Dec alone, and coord.PropagateEpoch reads them from the ICRS value, so
// propagating t.Coord itself moves nothing. Until #627 no position in the
// Resolver was ever moved, and a star faster than about 0.125″/yr never
// matched its own Gaia row, 16 years from SIMBAD's J2000.
func Coord(t resolve.Target) coord.ICRS {
	if t.PmRA == 0 && t.PmDec == 0 && t.Parallax == 0 && t.RadialVelocity == 0 {
		return t.Coord
	}

	return coord.NewICRSWithKinematics(t.Coord.RA(), t.Coord.Dec(), t.PmRA, t.PmDec, t.Parallax, t.RadialVelocity)
}

// Separation returns the angle between a, given at aEpoch, and b, given at
// bEpoch, with both at one epoch.
//
// When both carry a proper motion that epoch is J2000. When only one does, it
// is the other's own epoch, since a position without a motion cannot be
// moved: a catalog row that publishes none is compared where the star was
// when it was measured. When neither does, the positions are compared as
// given.
func Separation(a coord.ICRS, aEpoch time.Time, b coord.ICRS, bEpoch time.Time) (angle.Angle, error) {
	var err error

	switch {
	case moving(a) && moving(b):
		if a, err = coord.PropagateEpoch(a, aEpoch, time.J2000()); err != nil {
			return 0, fmt.Errorf("catalog: %w", err)
		}

		if b, err = coord.PropagateEpoch(b, bEpoch, time.J2000()); err != nil {
			return 0, fmt.Errorf("catalog: %w", err)
		}
	case moving(a):
		if a, err = coord.PropagateEpoch(a, aEpoch, bEpoch); err != nil {
			return 0, fmt.Errorf("catalog: %w", err)
		}
	case moving(b):
		if b, err = coord.PropagateEpoch(b, bEpoch, aEpoch); err != nil {
			return 0, fmt.Errorf("catalog: %w", err)
		}
	}

	return coord.Separation(a, b), nil
}

// moving reports whether c carries a proper motion to propagate it by.
func moving(c coord.ICRS) bool {
	return c.PmRA() != 0 || c.PmDec() != 0
}
