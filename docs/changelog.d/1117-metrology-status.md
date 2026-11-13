---
type: Fixed
pr: 1117
---
**A validation result with an unknown status is refused**: `internal/metrology` read a result document whose status
was none of its three outcomes, a misspelled or missing one included, and the baseline check, the baseline and the
generated accuracy table all treated it as measured.
