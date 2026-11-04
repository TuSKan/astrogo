---
type: Fixed
pr: 1011
---
**`lsk.Cache` removes a cached leap-second kernel that does not parse.** Its doc promised to remove corrupt or truncated files, but a corrupt cached kernel failed every call until deleted by hand, and the file opened for it leaked. The next call now fetches a fresh copy where downloads are consented to.
