---
type: Fixed
pr: 1006
---
**`ephemeris.Default()`'s doc now says which bodies it answers.** It claimed every named ID, but the planet centers of Mars through Pluto (NAIF 499–999) are `ErrUnsupportedBody`, since SOFA's theory places each system's barycenter. The doc names the barycenter IDs to use instead, and a test holds `Default` to the list.
