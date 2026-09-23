---
type: Fixed
pr: 380
---
**Six more tests that could not fail now can.** The skip guard caught a skip only
when it was the first statement after `if err != nil` and spelled `Skip`, so a
skip left after a classifier, and `metrology.NotVerified` — which skips after
recording — both got past it. Four accuracy suites recorded NOT VERIFIED on any
provider error, so a regression that broke the provider would have been
published as an outage. The guard now judges a skip by its enclosing block,
counts skipping helpers as skips, and flags a helper that swallows the error it
is handed; `testutil.UpstreamFailure` is exported so a suite can record before
it stops.
