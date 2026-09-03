---
type: Added
pr: 148
---
**The leap-second table is now validated, not just cross-checked.**
The complete 28-entry published ΔAT record is pinned and asserted against
gofa's table, including the half-open boundary convention at every step. A new
`validation`-tagged suite re-verifies it against the IERS timescale service —
the one reference that does not share ancestry with gofa, NAIF and finals2000A.
