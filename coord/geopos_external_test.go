package coord_test

import (
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/vector"
)

// geoPos keeps the position of an Instant's geometric state: the vector
// eph.Position returned before #803.
func geoPos(st eph.State, err error) (vector.Vec3, error) { return st.Pos, err }
