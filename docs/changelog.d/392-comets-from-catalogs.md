---
type: Added
pr: 392
---
**Comets on open orbits propagate from the catalogs.** `resolve.Target` carries
the comet form of its elements (`PerihelionDistance`, `PerihelionTime`), SBDB
decodes it, and `plan.FromCatalog` builds an orbit with e >= 1 from it instead
of dropping it to the kernel path. `mpcorb.Read` and `mpcorb.Open` read the
MPC's `CometEls.txt` too, all 959 comets, 118 of them on open orbits (#374).
