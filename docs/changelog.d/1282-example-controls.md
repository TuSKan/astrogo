---
type: Fixed
pr: 1282
---
**The nightly run of the README's network examples no longer fails on an outage of CelesTrak, MAST, JPL SBDB or Horizons.** It reports an example as not verified when a service it uses also fails a control request that cannot be wrong, and only SIMBAD had one; every API service an example reaches now has one, through the new `internal/testutil.HTTPControl`, and an untagged test keeps it that way.
