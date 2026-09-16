---
type: Changed — BREAKING
pr: 325
---
**`remote` is built on `io/fs`, and `gocloud.dev` is gone.** A storage container
is now an `fs.FS` and an open object a `remote.File` (`fs.File` + `io.ReaderAt` +
`io.Seeker`). `remote.Bucket` → `remote.FS`, `OpenBucket` → `OpenFS`,
`NewReaderAt` → `Open`, `Save` → `WriteFile`, and `IsNotFound` is deleted in
favour of `errors.Is(err, fs.ErrNotExist)`. Linked packages for a consumer of
`remote` fall from 406 to 219, with gRPC and OpenTelemetry — 98 packages
astrogo never configured — going to zero.
