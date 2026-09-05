package plan

import "github.com/TuSKan/astrogo/time"

// fixedEpoch is the instant tests use when they need *an* epoch rather than a
// particular one.
//
// See coord's copy for the full reasoning; in short, a test pinned to the wall
// clock drifts across the IERS measured/predicted EOP boundary — which moves
// every week — and eventually off the end of the file, failing on some future
// Tuesday with no code change. A past epoch is final and cannot drift.
//
// 2024-06-15 is inside measured EOP coverage, inside de440s's 1849-2150 span,
// and away from any leap second.
func fixedEpoch() time.Time {
	return time.Date(2024, time.June, 15, 12, 0, 0, 0, time.LocationUTC)
}
