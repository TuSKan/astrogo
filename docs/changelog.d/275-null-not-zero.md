---
type: Fixed
pr: 275
---
**A null table value read as 0.0.** `fits.BintableHDU`'s float-column accessor — and a
second copy of the same logic in `skybrightness/dataset/solar` — returned zero for an absent
value, so a missing flux became a flux of nothing and a missing magnitude became magnitude 0,
which is a very bright star. Nulls read as NaN now, which is what every consumer already
screens for.
