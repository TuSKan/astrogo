---
type: Fixed
pr: 208
---
**A 403 failed the build instead of skipping the test.** CelesTrak answers a
burst of requests with "Forbidden: Access is denied" and serves the same query
normally a minute later, which `testutil.SkipOnUpstreamFailure` classified as
astrogo sending a bad request. To a caller that sent no credential there is
nothing to correct, so it now skips; 401 still fails, and a new registry test
keeps the invariant that argument rests on (#206).
