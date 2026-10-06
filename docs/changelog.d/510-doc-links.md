---
type: Fixed
pr: 510
---
**59 Go doc links rendered as plain text**, most behind a removed or renamed
symbol, and `remote`'s and `ephemeris/satellite`'s package docs described APIs
that no longer exist. Both docs are rewritten, and `internal/docsguard` now
fails on a doc link that does not resolve.
