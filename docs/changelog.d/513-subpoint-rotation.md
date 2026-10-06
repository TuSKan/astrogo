---
type: Fixed
pr: 513
---
**`coord.SubPoint`, and with it `plan.SubsolarPoint`, `SublunarPoint` and
`Terminator`, ignored precession and nutation.** A GCRS direction was rotated
by GAST alone, putting the subsolar point 0.38° (42 km) off in 2026 and
growing 50″ a year. It now uses the full IAU 2006/2000A celestial-to-terrestrial
matrix, and the plan functions pass the apparent place.
