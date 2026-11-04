---
type: Fixed
pr: 983
---
**Cache tests in `passband`, `simbad`, `vizier`, `fink` and `sbdb` no longer fail when a request is retried.** Each pinned an exact count or list of requests to its test server, which a request retried after a timeout breaks. Now a call that must reach the service sends at least one request, a call served from the cache sends none, and a recorded sequence counts a back-to-back repeat once. `vizier`'s cache-key test also could not fail: its two tables' keys always differed in the declination's last digit, never in the table name.
