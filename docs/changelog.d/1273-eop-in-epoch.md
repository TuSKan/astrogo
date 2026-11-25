---
type: Changed — BREAKING
pr: 1273
---
**`epoch` loads Earth orientation itself**: the first `Time.EOP`, `Time.UTC` or `Time.UT1` that needs the IERS bulletin reads a pre-seeded copy or downloads it through `remote`, as offline mode and the download policy allow, with no import to add. Every program using `epoch` now links `remote`: a binary computing a Julian date is 6.1 MB, from 3.0 MB.
