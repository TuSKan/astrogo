---
type: Fixed
pr: 983
---
**Cache tests in `passband`, `simbad`, `vizier` and `fink` count requests per call, so a retried request no longer fails them.** Each pinned an exact total of requests to its test server, which a request retried after a timeout raises. Now a call that must reach the service sends at least one request, and a call served from the cache sends none. `vizier`'s cache-key test also could not fail: its two tables' keys always differed in the declination's last digit, never in the table name.
