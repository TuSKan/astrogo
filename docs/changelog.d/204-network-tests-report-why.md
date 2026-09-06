---
type: Fixed
pr: 204
---
**Five network tests failed the build on somebody else's outage and discarded
the error while doing it** — `Failed to resolve ISS` was the whole diagnostic
when CI throttled. They now route through `testutil.SkipOnUpstreamFailure` and
carry the error, and an `internal/docsguard` guard keeps the next one from
reintroducing it.
