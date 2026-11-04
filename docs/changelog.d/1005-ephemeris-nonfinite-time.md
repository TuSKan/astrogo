---
type: Fixed
pr: 1005
---
**`ephemeris.Default()` refuses a NaN or infinite time with the new `kepler.ErrNonFiniteTime`.** Every body it supports came back at such a time as a NaN position with no error, since SOFA's theories compute NaN from a NaN date and `Moon98` has no status to report it. The check is in `kepler.Provider.Locate`, so registered elements and any base provider are covered too.
