---
type: Fixed
pr: 160
---
**`Time.Sub` between two UTC epochs was short by every leap second between
them** — 27 s across 1972-2026, or 207 km of ISS track. It unified scales only
when they *differed*, so mixing scales gave the right answer and being
consistent gave the wrong one. Both operands now go through TT unless they
share a uniform scale (TAI/TT/TDB), where label arithmetic already is elapsed
time. `Sub` also saturates instead of wrapping past ±292 years — year 1 to 2026
used to return a negative duration — and rounds to the nearest nanosecond
rather than truncating (#149).
