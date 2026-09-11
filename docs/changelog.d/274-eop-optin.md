---
type: Changed — BREAKING
pr: 274
---
**IERS Earth-orientation data now needs `import _ "github.com/TuSKan/astrogo/remote/eop"`.**
The loader moved out of `remote` because it needs `remote.GetFile` and so cannot live in
the package that would have to import it back. Without the blank import, `Time.EOP`/`.UTC`/
`.UT1` report zero DUT1 and polar motion and log one warning — costing about an arcsecond
of topocentric position, measured on the Horizons corpus as a p50 of 1.4 arcsec becoming a
max of 8.8 arcsec.
