---
type: Added
pr: 0
---
`coord.LSRKinematic` is the kinematic Local Standard of Rest — what radio
spectroscopy means by "LSR", and what a spectral line's velocity is quoted
against unless a paper says otherwise. It joins the two dynamical kinds in
`LSRCorrection` and `LSRApex`. Unlike them it is published as an apex rather
than as Galactic components — 20 km/s toward RA 270°, Dec +30°, **B1900
equinox** (Gordon 1975) — so astrogo derives the ICRS vector from that
statement rather than copying a converted one. The B1900→B1950 step uses IAU
1976 precession where Newcomb's is correct, which is measured rather than
assumed: 0.55″ of direction and 5.4 cm/s against Astropy's independent
realisation of the same definition. Closes [#295].
