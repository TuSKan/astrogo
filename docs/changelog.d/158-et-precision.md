---
type: Fixed
pr: 158
---
**ET was quantised to ~40 microseconds.** `lsk.TDBToET(lsk.UTCToTDB(...))`
summed the two-part Julian Date before subtracting J2000, and one ULP at a
modern Julian Date is 40 µs — 4 cm of lunar motion, at the level of the 33 mm
claimed against Horizons. The new `lsk.UTCToET` removes the epoch first and
resolves **0.128 µs**, a 256-fold improvement; `TDBToET` is deprecated.
