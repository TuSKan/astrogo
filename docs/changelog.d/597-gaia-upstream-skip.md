---
type: Fixed
pr: 597
---
**`catalog/gaia`'s `TestArchivesAgree` failed on ESA's own query timeout** (HTTP 408, "Job timeout/aborted"). It now skips on every upstream failure `testutil.SkipOnUpstreamFailure` recognizes, as the other network tests do.
