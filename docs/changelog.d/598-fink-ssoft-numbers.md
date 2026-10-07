---
type: Fixed
pr: 598
---
**`catalog/fink` read every asteroid number in FINK's SSOFT bulk table as 0**, because the table stores `sso_number` as a string. The index held one entry, `Count()` returned 1, and whenever the single-object endpoint failed, `Resolve` reported a numbered asteroid as not found. All 148,922 usable rows are indexed now.
