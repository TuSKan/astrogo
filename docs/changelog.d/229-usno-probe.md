---
type: Fixed
pr: 229
---
**The USNO comparison suite had been silently skipping.** A single-shot TCP
probe cached its failure in a `sync.Once`, so one dropped packet retired all
fourteen `TestUSNO_*` functions for the rest of the binary — as SKIP, which
reads as a pass. The probe now goes through `testutil.Reachable` (which retries
over IPv4) and remembers only success; a new docsguard check stops the next
hand-rolled probe (#225).
