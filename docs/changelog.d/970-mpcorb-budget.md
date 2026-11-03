---
type: Fixed
pr: 970
---
**`catalog/mpcorb`'s consent test checks the download budget it said it checked.** Its doc comment promised that a grant too small for the file is refused as clearly as no grant at all, but no part of the test granted too small a budget, and its no-grant step passed on any error. It now requires `ErrDownloadDenied` both without a grant and under a 1 KiB one, which only the size the MPC reports can refuse, and that nothing is cached.
