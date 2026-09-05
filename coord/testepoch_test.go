package coord_test

import "github.com/TuSKan/astrogo/time"

// fixedEpoch is the instant tests use when they need *an* epoch rather than a
// particular one.
//
// # Why not time.NowUTC
//
// A test pinned to the wall clock drifts on its own schedule and fails on some
// future Tuesday with no code change — the worst kind of failure, because the
// first suspicion falls on a recent commit. The near-term mechanism is Earth
// Orientation Parameters: IERS finals2000A carries measured values up to about
// a month behind the present and predictions for roughly a year ahead, and
// that boundary moves every week. A test at "now" crosses from measured to
// predicted, and eventually past the end of the file, silently changing the
// numbers it computes.
//
// A past epoch cannot drift. Measured EOP for a date already behind us is
// final and will not be revised, so this instant computes the same values in
// 2027 as it does today.
//
// # Why this one
//
// 2024-06-15 is comfortably inside measured EOP coverage, inside de440s's
// 1849-2150 span, and far from any leap-second boundary — so a test using it
// is exercising ordinary conditions, and any boundary case is a deliberate
// epoch chosen at its own call site rather than an accident of when the suite
// happened to run.
func fixedEpoch() time.Time {
	return time.Date(2024, time.June, 15, 12, 0, 0, 0, time.LocationUTC)
}
