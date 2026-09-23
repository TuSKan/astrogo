---
type: Fixed
pr: 379
---
**Horizons outages no longer fail the validation suite.** The Horizons fetchers
did not read the HTTP status, so the 503 JPL serves under load reached callers
as a sentinel no classifier could see; the corpus generator also turned a
partial fetch into a reported "corpus would change". Each fetcher now surfaces
the status, the generator skips on an outage instead of diffing half a fetch,
and four unclassified kernel fetches were found by auditing per call site
rather than per file.
