---
type: Fixed
pr: 462
---
**`fits.Read` rejects unusable structural keywords** instead of trusting
them: a missing or malformed `NAXIS2` no longer reads as an empty table, and a
negative axis, an overflowing axis product or `NAXIS` above 999 is an error
rather than a panic or an empty image (#460).
