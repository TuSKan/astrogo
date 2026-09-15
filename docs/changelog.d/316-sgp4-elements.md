---
type: Added
pr: 316
---
`ephemeris/satellite/sgp4` — a public SGP4 package written from Vallado's
published algorithm, starting with its element-set layer: `Elements`,
`ParseTLE`/`ParseTLEName`, `VerifyTLEChecksums` and the `Gravity` models.
Checksum verification is a separate call from parsing, which is what makes
Vallado's three deliberately-bad-checksum cases readable and takes astrogo's
coverage of its own reference suite from 30 of 33 to 33 of 33. No propagation
yet — see `docs/sgp4.md`. [#310]
