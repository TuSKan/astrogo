---
type: Fixed
pr: 158
---
**ET was quantised to ~40 microseconds.** The Julian-date conversion pair
summed the two-part Julian Date before subtracting J2000, and one ULP at a
modern Julian Date is 40 µs — 4 cm of lunar motion, at the level of the 33 mm
claimed against Horizons. The new `lsk.UTCToET` removes the epoch first and
resolves **0.128 µs**, a 256-fold improvement. `UTCToTDB` and `TDBToET` are
removed: they held the same value in a container that could not represent it,
and nothing in production called them.
