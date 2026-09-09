---
type: Fixed
pr: 240
---
`plan`'s tests no longer revoke the download consent their own `TestMain`
grants: the four blanket `remote.Reset` cleanups are now scoped
`remote.Capture(...).Restore`, and a new guard in `internal/docsguard` fails
any consent-granting package that reintroduces one (#240).
