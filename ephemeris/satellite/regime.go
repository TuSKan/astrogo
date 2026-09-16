package satellite

// Verified reports whether this element set sits inside the regime astrogo's
// SGP4 verification actually covers, and says why when it does not.
//
// Deprecated: it now returns true for every element set that can be
// constructed, because the regime it warned about no longer exists. See below
// before removing a call to it; there may be nothing to replace it with.
//
// # What it used to mean, and what happened to that
//
// astrogo's propagation was measured against Vallado's reference suite (AIAA
// 2006-6753) and seven of the thirty readable cases did not reproduce it, by
// 0.22 km to 3438 km. Every large one sat below a perigee of 220 km, where
// SGP4 switches to its simplified drag model, so this predicate reproduced the
// model's own branch and warned a caller off that band. Against the thirty
// cases it flagged ten, seven of which diverged.
//
// The divergences were not a property of SGP4. They were one digit in the Go
// implementation astrogo depended on — 128 where the algorithm says 120, in
// the s⁴ atmospheric-density coefficient that feeds the secular drag term
// (#309). With the model written from Vallado's published algorithm instead,
// all thirty-three cases agree to a maximum of 4.1e-06 km, the low-perigee
// band included: 28350 went from 3438.51 km to 6.3e-09 km.
//
// So there is no longer a regime to flag. Returning false for a low-perigee
// orbit would now be false: those are among the best-agreeing cases in the
// suite.
//
// # Why it returns true rather than being deleted
//
// Because "this element set is outside what has been verified" is a question
// worth being able to ask, and a caller asking it should get an answer rather
// than a compile error while the replacement is decided. It is marked
// deprecated under this repository's policy — two minor releases before
// removal — and if a real regime is ever identified again it comes back with a
// measurement behind it rather than as a guess.
//
// A caller who wants the model's own branch predicates, rather than a verdict
// about astrogo, should use the propagator directly:
// [Satellite.Propagator] exposes
// [github.com/TuSKan/astrogo/ephemeris/satellite/sgp4.Propagator.SimplifiedDrag],
// [github.com/TuSKan/astrogo/ephemeris/satellite/sgp4.Propagator.DeepSpace] and
// [github.com/TuSKan/astrogo/ephemeris/satellite/sgp4.Propagator.PerigeeAltitude].
// Those describe the orbit, which is a durable question, where this described
// astrogo's own coverage, which was not.
func (s *Satellite) Verified() (bool, string) { return true, "" }
