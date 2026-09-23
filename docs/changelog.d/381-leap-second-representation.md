---
type: Fixed
pr: 381
---
**A leap second is now an instant of its own.** `Date(2016, 12, 31, 23, 59, 60, …)`
used to land on the following midnight and convert with ΔAT 37, where the inserted
second carries 36 — a full second wrong. UTC Julian Dates now follow SOFA's
convention, the fraction of a day ending in a leap second being of 86401 seconds,
restated against astrogo's own leap-second table so a registered one is honoured.
Ordinary days are unchanged to the bit. **Do not subtract two UTC Julian Dates
across such a day**: the difference is neither the labels nor the elapsed time;
use `Time.Sub`, or `ToGo` for labels. Four places in this repository did, and are
fixed: UT1 in `coord.Context`, SGP4's `AtTime` and epoch, and `lsk.UTCToET`.
