---
type: Fixed
pr: 305
---
**The README's frame list was three frames out of date.** It named ITRS, TETE
and LSR as missing while all three were on `main`, which is the worst kind of
stale documentation — the honest-limitations paragraph is the one a reader
trusts most. It now lists what arrived and what did not, and says why LSRK has
not: its apex is published in the B1900 equinox that `coord.FK4` cannot express.
The `coord` row of the package map enumerates the frames the way the `time` row
already enumerates its scales.
