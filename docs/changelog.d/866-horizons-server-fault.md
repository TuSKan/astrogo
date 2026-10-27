---
type: Fixed
pr: 866
---
**A fault on JPL Horizons' own server is recognized on every path to it.** The
name lookup in `catalog/jpl` wraps the same sentinel as kernel generation, and
the live tests skip on that outage instead of failing.
