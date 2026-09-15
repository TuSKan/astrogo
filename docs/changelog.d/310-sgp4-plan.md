---
type: Added
pr: 310
---
`docs/sgp4.md` — the design for a from-scratch SGP4 at
`ephemeris/satellite/sgp4`, and the measurement behind it: the divergences
astrogo's Vallado suite has been reporting are one transcription error in the
current dependency, `128.0` where the algorithm says `120.0` in the s⁴ drag
coefficient. [#309]
