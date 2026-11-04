---
type: Fixed
pr: 1007
---
**`ephemeris.Default()` acts on SOFA's positive statuses.** A `Plan94` solution that failed to converge was returned as a position, NaN at JD 1e12, and is now `ErrSofaFailure`. A date outside `Epv00`'s 1900–2100 or `Plan94`'s 1000–3000 is still answered, but the first one in a process writes a warning through `logging` naming the theory, its span and how SOFA says accuracy falls off. `Default`'s doc states the spans.
