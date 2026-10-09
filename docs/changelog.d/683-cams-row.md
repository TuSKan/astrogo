---
type: Fixed
pr: 683
---
**`VALIDATION.md` ticked CAMS aerosol optical depth as validated, but every test behind it skips.** The orientation test needs an `s3://` backend, which astrogo has not had since gocloud.dev's removal, and the ground-truth tests need licensed files absent from CI. The row now says it is not run and dates its figures, and the skip message no longer says the backend is being rebuilt.
