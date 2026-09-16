---
type: Changed — BREAKING
pr: 328
---
**`coord.Apparent` no longer exists — it is `coord.CIRS` now, with its
transforms.** The name said the one thing the type is not: "apparent place" has meant the equinox-based
place — true equator, true equinox of date — for two centuries, and this is the
CIRS place, which measures right ascension from the Celestial Intermediate
Origin. They are apart by the equation of the origins: **20.3 arcminutes in
2026**, growing 46 arcseconds a year, in right ascension only — so a caller
comparing against an almanac sees a pure RA offset and reaches for a
sidereal-time bug. `coord.TETE` is the equinox-based place and keeps the word
everyone else means by it.

Migration is mechanical: `Apparent` → `CIRS`, `NewApparent` → `NewCIRS`,
`AstrometricToApparent` → `AstrometricToCIRS`, `ApparentToObserved` →
`CIRSToObserved`, `ApparentToTETE` → `CIRSToTETE`, `TETEToApparent` →
`TETEToCIRS`. `Name()` now returns `"CIRS"` and `String()` is prefixed `CIRS`.
A program that stays inside the pipeline is otherwise unaffected — the numbers
do not change. Closes [#298].
