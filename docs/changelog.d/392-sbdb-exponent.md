---
type: Fixed
pr: 392
---
**SBDB values written in E-notation are read whole.** The decoder kept the
digits before the `E`, so a mean anomaly of `-2.593805408851336E-5` degrees
was read as −2.59°: every element set near perihelion was off by orders of
magnitude.
