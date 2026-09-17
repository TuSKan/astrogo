---
type: Fixed
pr: 349
---
**A network failure is no longer swallowed by the deadline it caused.**
`internal/testutil.Unreachable` excluded `context.Canceled` and
`context.DeadlineExceeded` before every other check, so an unreachable host
reported as reachable whenever the endpoint's own download timeout fired
alongside the dial failure — which is how `ephemeris/jpl`'s kernel tests went
red on NAIF's downtime with their skip guard in place and not firing. The
exclusion now sits ahead of the check that genuinely cannot tell a network
timeout from a caller's deadline, and behind the three that can: a DNS failure,
a failed dial and `ECONNREFUSED` are not things a context error produces, so a
deadline alongside them decides nothing. Four `plan` tests that fetch a DE44x
kernel before testing AstroPixels, NASA or USNO gained the guard they never had,
and `newEph` now skips rather than substituting the analytic ephemeris when NAIF
is unreachable — that substitution made `TestUSNODecomposesTheTopocentricBias`
report a −0.456″ declination bias as evidence of a precession-nutation defect
that does not exist. Closes [#348].
