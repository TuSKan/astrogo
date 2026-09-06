---
type: Added
pr: 186
---
**Every parser that reads bytes astrogo did not produce is now fuzzed** — `fits`
(3 targets), `ephemeris/satellite` (2), `catalog/norad`, `internal/votable` and
`time/internal/iers`, joining the existing SPK targets. Seeds are literals, so the
corpora run in ordinary CI; two crashers they found are checked in as regressions (#139).
