---
type: Fixed
pr: 148
---
**Two ephemeris providers silently reinterpreted the caller's time scale.**
SGP4 read the calendar fields raw, putting the ISS 530 km out for a TT input;
the JPL provider treated anything but TDB as UTC, worth 40 arcsec of lunar
motion. Both now normalise at the entry point, and a new contract test asserts
every provider returns the same state however the instant is labelled.
