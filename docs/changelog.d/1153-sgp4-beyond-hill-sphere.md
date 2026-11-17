---
type: Fixed
pr: 1153
---
SGP4 refuses a propagated position beyond the Earth's Hill sphere with the new `sgp4.ErrBeyondHillSphere`. Run thousands of years from its epoch, an element set's orbit grew without bound and came back as a finite position, 4.5e24 km away, with no error.
