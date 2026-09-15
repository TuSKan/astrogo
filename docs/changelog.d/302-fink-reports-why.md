---
type: Fixed
pr: 302
---
**The FINK validation test now says why it could not compare.** "No valid
r-band observations" covered two conditions that want opposite outcomes — FINK
holding no r-band photometry for the object right now, which is absence, and
FINK renaming a column, which is a schema change astrogo must notice. The test
now skips for the first and fails for the second, naming the columns and the
counts either way.
