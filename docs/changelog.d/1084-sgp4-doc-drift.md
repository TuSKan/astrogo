---
type: Fixed
pr: 1084
---
**`docs/sgp4.md` no longer names an `ErrSubOrbital` sentinel**, which never existed: the reference disabled Vallado's
error 5, and a sub-orbital element set builds and reports `sgp4.ErrDecayed`, now pinned by a test. It also no longer
says `sgp4.Propagator.AtTime` uses Julian Dates, which it stopped doing in #144.
