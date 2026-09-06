---
type: Fixed
pr: 180
---
**Seven predicates could not report a failure, so an error read as "the answer
is no".** The scheduler's and visibility solver's bisection predicates returned
a bare `false` for a constraint that *could not be evaluated*, so a target
silently vanished from a schedule; `ObservableWindows` swallowed a failure
during refinement, moving a rise/set boundary rather than dropping it; and
`DiffuseGalacticLight.capFactor` treated a star map that could not answer as a
sightline with no starlight, quietly dropping the Toller cap. All now
propagate (#177).

**`plan.VisibleTonight` reports an incomplete result instead of only logging
it.** It still skips a candidate it cannot evaluate rather than failing the
night, but now returns its results alongside an error wrapping the new
`plan.ErrIncomplete`, naming every catalogue source, target, small body, moon
kernel and candidate that was dropped and why. A caller who ignores that error
gets the previous behaviour; one who checks it can finally tell a quiet sky
from an unreachable JPL (#177).

**`satellite.ValidateTLE` now checks that every numeric field is numeric, which
prevents the SGP4 backend from calling `os.Exit` on the caller's process.**
`joshuaferrara/go-satellite` parses the twelve numeric TLE fields through
helpers that `log.Fatal` on a parse error — no error, no panic, nothing to
recover. A TLE's modulo-10 checksum cannot catch this, since letters and spaces
contribute nothing to the sum, so a field replaced by text can still check out.
`NewFromTLE` now refuses such a set with `ErrMalformedTLE` naming the field, and
`Satellite.MeanMotion` comes from that same parse rather than a separate one
that returned a silent `0`.

**`cams.isDimensionScale` reported an unreadable object header as "not a
dimension scale".** A corrupt file then filed its axes as data variables, so
the reader indexed a shape the file does not have and the failure surfaced much
later as a missing dimension on a variable whose dimensions are all present. It
was the one attribute reader #172 left behind.
