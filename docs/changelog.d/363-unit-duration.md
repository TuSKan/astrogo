---
type: Added
pr: 363
---
**`unit.Duration`**, an elapsed time stored in seconds, with `Seconds`,
`Minutes`, `Hours`, `Days` and `JulianYears` constructors and matching
accessors. `unit.JulianYear` joins the unit table as 365.25 days exactly.

It is the third named scalar after `Length` and `Velocity`, and the one they
implied: `Velocity` is declared as `Meter.Div(Second)`, so the package has been
doing time arithmetic since it was written without a type for it.

`time.ToGoDuration` and `time.FromGoDuration` convert to and from the standard
library's `time.Duration`, which remains what a timeout, a ticker or a sleep is
measured in. `ToGoDuration` reports whether the value fit, since an int64
nanosecond count stops just past ±292 years.
