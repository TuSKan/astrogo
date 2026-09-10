---
type: Fixed
pr: 272
---
**Staging inside the cache bucket broke concurrent processes on Windows**, which
#266 shipped and CI then failed on: two `go test` binaries sharing one cache
collided on a staging path they could not retry past, failing every test in
`ephemeris/jpl` for three minutes. The parameter is gone. The in-process write
lock is keyed on the key's basename instead — the collision domain the staging
path actually has — which covers the separate-bucket case the parameter was
added for (#241).
