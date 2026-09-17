---
type: Changed
pr: 351
---
`docs/VALIDATION.md`'s generated accuracy table is regenerated from a full
`validation` + `network` collection. It was dated 2026-08-30 and had drifted in
three ways: ten suites had grown their corpora (the seven SOFA planets from
N=516 to 1204, the two time-scale round trips from 120 to 432 and 180 to 540),
four suites were being measured and never published, and the Horizons-referenced
rows had moved. `coord.topocentric.vs_sofa.stepwise` and `.collapsed` now appear
at 0.000″ across all four statistics over 210 combinations, and
`ephemeris.astrometric.geocentric` and `.apparent.geocentric` are cited by suite
name rather than by test file. Every row is still ✅ verified and inside its
contract. The status table's claim that the two SOFA-comparison suites were "not
yet in the generated table" is no longer true and is corrected.
