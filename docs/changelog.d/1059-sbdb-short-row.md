---
type: Fixed
pr: 1059
---
**A short row in SBDB's bulk query answer panicked `sbdb.SearchBright`, and a missing column read another
column's value.** Such cells now read as absent, an answer without its identity columns is refused, and the
decoder has a fuzz target.
