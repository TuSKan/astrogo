---
type: Fixed
pr: 0
---
**JPL being unreachable no longer turns a build red.** `TestSmallBodyEros` and
`TestSmallBodyMultiMatch` are untagged and hit the live network, and their own
doc comment said they must not fail for somebody else's downtime — but they
recognised only the two ways Horizons answers 200 and still cannot serve a
kernel. A CI run timed out dialling NAIF's file server, which is a different
host, and the build failed. `internal/testutil.Unreachable` is the predicate
they were missing: a timeout, a DNS failure or a dial that never reached a
service, distinguished from any status a server actually returned.
