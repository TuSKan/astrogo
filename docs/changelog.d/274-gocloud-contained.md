---
type: Changed
pr: 274
---
**Only `remote/file` names the storage library now.** `remote.IsNotFound` replaces
`gcerrors.Code(err) == gcerrors.NotFound` at its three call sites and
`internal/testutil.BucketKeys` takes an `fs.FS`, so `gocloud.dev` appears in no import
outside `remote/file` — enforced by `TestGocloudStaysInsideRemoteFile`. The driver has
been swapped once already; what made that expensive was every package that had an opinion
about it.
