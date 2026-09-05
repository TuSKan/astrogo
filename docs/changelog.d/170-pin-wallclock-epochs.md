---
type: Fixed
pr: 170
---
**Six test files asserted astronomical results at whatever instant the suite
happened to run.** A test at `time.NowUTC()` drifts across the IERS
measured/predicted EOP boundary — which moves every week — and eventually off
the end of the file, failing on a future date with no code change. Those now
use a fixed past epoch, which is final and cannot drift.
`TestNoUndeclaredWallClockTests` scans the module and requires any remaining
wall-clock test to declare why the present is its subject; twelve do, and a
stale declaration fails too (#142).
