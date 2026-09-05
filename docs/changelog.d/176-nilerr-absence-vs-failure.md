---
type: Fixed
pr: 176
---
**Three places reported a real failure as a legitimate absence.** The CAMS
HDF5 reader treated *any* `ReadAttribute` error as "attribute missing", so a
corrupt file reported every attribute as absent and the reader carried on with
defaults; it now asks which attributes exist first, which separates absence
from an unreadable header. `IntegratedStarlight` read `err != nil || value <= 0`
and reported a map that could not answer — a band it does not carry — as an
uncovered direction; the two are now distinguished by the new
`skybrightness.ErrNoCoverage` (#172).
