---
type: Fixed
pr: 774
---
**`starlight.FetchBrightStars` gathers every bright Gaia source past the archive's row cap.** The archive now caps a synchronous answer at 50,000 rows, the bright-star query matches 177,426, and the capped CSV looked complete, so some 10,900 Hipparcos stars were reported missing from Gaia instead of 74, light `AddBrightStars` would have counted twice. The query now reads a VOTable and halves any truncated magnitude interval. The VOTable reader also missed a `QUERY_STATUS` of `OVERFLOW` written after the table, where TAP services put it, so it never reported a truncated answer.
