# Storage: `io/fs` instead of gocloud.dev

A plan for replacing `gocloud.dev/blob` with the standard library's own
filesystem abstraction, and deleting the dependency.

This document is the design. It was meant to be argued with before any of it was
built.

**Status.** PRs 1-4 have landed: the `io/fs` core, the `file://`, `http`,
`https` and `mem://` backends, and the switchover that removed `gocloud.dev`
and `gocloud-ext` from `go.mod`. The §1 table is re-measured below. PR 5 --
`s3`, `gcs`, `azblob` and `sftp` on their own SDKs -- is outstanding, so those
schemes are unregistered in the meantime.

---

## 1. What it costs today, measured

Counting *linked* packages — `go list -deps`, so what a consumer's binary
actually contains:

| import | total | gRPC | OpenTelemetry | gocloud |
| :--- | ---: | ---: | ---: | ---: |
| `astrogo/time` | 93 | 0 | 0 | 0 |
| `astrogo/coord` | 100 | 0 | 0 | 0 |
| `astrogo/ephemeris` | 105 | 0 | 0 | 0 |
| **`astrogo/remote`** | **406** | **64** | **34** | **9** |
| **`astrogo/plan`** | **433** | **64** | **34** | **9** |

And after, measured the same way on the same machine once PR 4 landed:

| import | total | gRPC | OpenTelemetry | gocloud |
| :--- | ---: | ---: | ---: | ---: |
| `astrogo/time` | 93 | 0 | 0 | 0 |
| `astrogo/coord` | 100 | 0 | 0 | 0 |
| `astrogo/ephemeris` | 105 | 0 | 0 | 0 |
| **`astrogo/remote`** | **219** | **0** | **0** | **0** |
| **`astrogo/plan`** | **246** | **0** | **0** | **0** |

The scientific packages are unchanged, which is the point: they never carried
any of it, and the tripling only ever happened at `remote`. That tripling is
gone — 187 packages off `remote` and 187 off `plan`, including all 98 gRPC and
OpenTelemetry ones.

The scientific packages carry nothing. The moment a consumer reaches `remote` —
and `plan` does, for EOP and JPL kernels — the graph triples.

It is not the cloud drivers, which are already opt-in subpackages exporting
nothing. `gocloud.dev/blob` **alone** is 388 packages, 141 of them heavy;
`fileblob` and `httpblob` add four between them. `blob.go` imports
`gocloud.dev/internal/otel` unconditionally, and that shim is the weight: 385
packages so that a library can emit traces astrogo never configures an exporter
for. There is no upstream escape — v0.46.0 is the latest release, the shim is
not behind a build tag, and it is not `gocloud-ext`'s doing either.

---

## 2. The shape: `fs.FS`, and three interfaces astrogo defines

`io/fs` is the standard library's answer to exactly this problem, and the
reason to prefer it over any third-party abstraction is not weight. It is that
**a great deal of Go already speaks it**: `os.DirFS`, `embed.FS`,
`archive/zip`, `testing/fstest`, `fs.WalkDir`, `fs.Glob`, `fs.Sub`,
`fs.ReadFile`. An abstraction nobody else implements buys nothing; this one is
already implemented by everything.

### 2.1 The public type

```go
// remote.File is an open object. fs.File gives Read, Stat and Close; the two
// additions are what a caller needs to seek inside a 3 GB SPK kernel without
// reading the first 2.9 GB of it.
type File interface {
    fs.File
    io.ReaderAt
    io.Seeker
}
```

That is the whole read model. The exported `remote.ReaderAt` no longer exists;
its chunked LRU — 64 KiB × 16, 1 MiB resident for any object — became a
decorator over `io.ReaderAt` inside `remote.Open`, and kept its benchmark.

### 2.2 What `io/fs` does not have, and the three interfaces that answer it

`io/fs` is read-only and has no context. Both gaps are filled the way the
standard library fills its own — small optional interfaces, tested for with a
type assertion — which is also how [`s3iofs`](https://github.com/wolfeidau/s3iofs)
does it (`RemoveFS`, `WriteFileFS`).

```go
// CreateFS is a filesystem that can be written to, one stream at a time.
//
// Streaming, not WriteFile([]byte): a DE440 kernel is 3 GB and astrogo's whole
// download path exists so that it never has to fit in memory. s3iofs offers
// WriteFile and astrogo cannot use it for this.
type CreateFS interface {
    fs.FS
    Create(name string) (io.WriteCloser, error)
}

// RemoveFS deletes. Same signature as s3iofs's, deliberately.
type RemoveFS interface {
    fs.FS
    Remove(name string) error
}

// ContextFS binds a filesystem to a context, returning one whose operations
// honour it.
type ContextFS interface {
    fs.FS
    WithContext(ctx context.Context) fs.FS
}
```

`fs.StatFS`, `fs.ReadDirFS` and `fs.SubFS` come from the standard library and
need nothing from astrogo.

### 2.3 The context problem, which is the real design question

`fs.FS.Open(name string)` takes no context. That is fine for a local disk and
not fine for a 3 GB download over a network, and CLAUDE.md is emphatic that
every astrogo entry point takes `ctx` first.

**Measured, this is not hypothetical**: `s3iofs` calls `context.TODO()` and
`context.Background()` in nine places. Used as published, every S3 request it
makes is uncancellable.

The answer is that **the filesystem value carries the context**, constructed per
operation, which is why `ContextFS` exists. astrogo's own API is unchanged —
`GetFile(ctx, …)` still takes it first — and internally:

```go
fsys := reg.open(endpoint.URL)
if cfs, ok := fsys.(ContextFS); ok {
    fsys = cfs.WithContext(ctx)
}
```

For `s3iofs` specifically the seam is its exported `S3API` interface and
`NewWithClient`: astrogo supplies a client that substitutes the caller's context
for the one it is handed. Roughly fifteen lines, and it makes an uncancellable
library cancellable without forking it.

**A backend that does not implement `ContextFS` is refused for any
`Downloadable` endpoint**, and that is a check rather than a convention. A
multi-gigabyte fetch that cannot be cancelled is not something to discover at
runtime.

### 2.4 Paths

`fs.ValidPath` requires unrooted, `/`-separated, no `.` or `..` elements — which
is *exactly* the key rule CLAUDE.md already mandates and `path.Join` already
produces. The standard library starts enforcing a rule astrogo has been keeping
by hand.

---

## 3. The backends

| scheme | implementation | SDK |
| :--- | :--- | :--- |
| `file://` | `os.Root` (Go 1.24+) + staging rename | none |
| `http://`, `https://` | `remote/api`'s client, range requests | none |
| `mem://` | `fstest.MapFS` + a write shim | none |
| `s3://` | `s3iofs` + the context adapter of §2.3 | AWS |
| `gs://`, `azblob://`, `sftp://` | the same shape, per vendor | each |

Three of the five that ship in core have no SDK behind them at all. The cloud
ones stay third-party and stay in the opt-in subpackages they already live in,
exporting nothing and registering by blank import — reimplementing S3 request
signing is exactly what the rest of this reasoning argues against.

`os.Root` is worth taking for the local backend: it confines every operation
beneath a directory at the OS level, which is a stronger guarantee than the
path checks astrogo does today, and it is the reason the `file` backend can be
short.

---

## 4. What this deletes

Beyond the dependency:

- **The scheme registry shrinks to a map of `func(*url.URL) (fs.FS, error)`.**
  No `URLMux`, no `BucketSchemes`, no `ValidBucketScheme`.
- **`?prefix=` becomes `fs.Sub`**, which is in the standard library.
- **The memory bucket becomes `fstest.MapFS`.** Tests that build a fake bucket
  today are three lines.
- **`Copy`, `Exists`, `ReadAll`, `WriteAll` stop being methods** and become
  `fs.ReadFile` and small helpers, because that is what they are.

And one thing it fixes rather than deletes:
[#315](https://github.com/TuSKan/astrogo/issues/315), the two-process staging
collision, is unfixable while the staging strategy is `fileblob`'s. `fileblob`
names its staging file from `time.Now().UnixNano()` and the Windows clock does
not advance — measured, one distinct value across 2000 consecutive reads.
astrogo has worked around it twice (#241, #307) and `writelock.go`'s own comment
states the boundary it cannot reach. Owning the local backend makes the fix
ordinary: stage under a name containing the process id and a counter, which is
what every other implementation does.

---

## 5. What it is expected to buy

Estimated, to be measured rather than claimed:

- `astrogo/plan` from **433 linked packages to roughly 283** — a third of the
  graph — for a consumer that does not import `catalog/fink`.
- **This does not remove gRPC from a FINK user**: Arrow pulls its own,
  independently. Stating that here stops the headline overstating itself later.
- [#109](https://github.com/TuSKan/astrogo/issues/109) — astrogo inherits a
  patch-level `go` directive from `gocloud-ext`, forcing a toolchain download in
  every CI job — resolved by the same change.
- `gocloud-ext` stops being load-bearing for astrogo. Its `httpblob` becomes an
  `fs.FS` inside astrogo; whether the repo keeps a life of its own is then a
  free choice rather than a dependency.

---

## 6. The public API changes, and that is the point

This is not a swap behind a stable façade. `remote.Bucket` no longer exists: it
was a gocloud concept and it went.

| today | after |
| :--- | :--- |
| `remote.Bucket` (no longer exists) | `fs.FS` (+ the three interfaces of §2.2) |
| `remote.OpenBucket(ctx, url) (*Bucket, error)` | `remote.OpenFS(ctx, url) (fs.FS, error)` |
| `remote.NewReaderAt(ctx, bucket, key)` | `remote.Open(ctx, fsys, name) (File, error)` |
| `remote.Save(ctx, bucket, key, r)` | `remote.WriteFile(ctx, fsys, name, r)` |
| `remote.GetFile(ctx, id, name, …) (*Bucket, string, error)` | `(fs.FS, string, error)` |
| `remote.IsNotFound(err)` | `errors.Is(err, fs.ErrNotExist)` |

That last row is the one worth pausing on. `remote.IsNotFound` no longer exists.
It existed because gocloud had its own error codes; with `io/fs` the answer is
`errors.Is(err, fs.ErrNotExist)`, which every Go programmer already knows and
which works across `os`, `embed`, `zip` and every backend here. The function is
deleted rather than kept as a wrapper.

No deprecations and no compatibility shims: pre-1.0, and a façade over a
different model is how the model leaks anyway.

---

## 7. Delivery

Each step green and independently reviewable, branched from `main` once its
predecessor merges.

| PR | contents | how it is judged | |
| :-- | :--- | :--- | :-- |
| **1** | This document. | Is the plan right? | landed |
| **2** | `remote/file`: the `File` type, the three interfaces, the scheme registry, and the `file://` backend on `os.Root`. Alongside gocloud, not replacing it. | #315's two-process reproduction fails before and passes after. `fstest.TestFS` passes against the backend. |
| **3** | `http`/`https` and `mem://`. | Range behaviour matches today's; `BenchmarkReadAtStrategies` re-run and recorded, not assumed. | landed |
| **4** | Switch `remote` over, delete the gocloud path, drop `gocloud.dev` from `go.mod`, and reshape the public API per §6. | The §1 table re-measured. Every consuming package's tests unchanged except where §6 renames a call. | landed |
| **5** | `s3` (on `s3iofs` + the context adapter), then `gcs`, `azblob`, `sftp`. | Each exports nothing, registers by blank import, and refuses to serve a `Downloadable` endpoint unless it implements `ContextFS`. | pending |

PR 2 carried #315 deliberately: it is the clearest evidence that this layer
should be astrogo's, and fixing it first meant the rest was measured against a
defect already closed.

---

## 8. Risks

**`fstest.TestFS` is the acceptance test, and it is strict.** It checks
`ValidPath` handling, `ReadDir` ordering, `Sub`, `Glob`, `Stat` consistency and
more. Every backend must pass it. That is a much stronger bar than the current
suite and is the main reason to expect this to take longer than it looks.

**`s3iofs` is a small dependency with an uncancellable core.** §2.3's adapter
handles it, but it is worth being clear that astrogo would be relying on a
library whose published behaviour needs working around. The alternative — an
astrogo S3 filesystem over the AWS SDK directly — is maybe 200 lines and
removes the workaround. That is a decision for PR 5, and the plan does not
pre-commit to it.

**Writes are not local-only today.** `SetDataDir` documents
`s3://my-cache-bucket` and `sftp://host/path` as valid cache locations, so the
write path must work over every scheme, not just `file://`. If that capability
is not actually wanted, saying so would simplify PR 5 considerably — the cloud
backends would become read-only, and `CreateFS` would be needed on `file://`
alone.

---

## 9. Explicitly not in scope

- **Removing Apache Arrow.** It is `catalog`'s columnar cache, it earns its
  place, and it is a separate question.
- **Reimplementing S3/GCS/Azure request signing.**
- **A general-purpose storage abstraction for other projects.** The public
  surface is `remote`'s, and `fs.FS` is the standard library's — astrogo adds
  three interfaces and no more.
- **Telemetry of astrogo's own.** The point is not to replace gocloud's traces
  with ours. `logging` is the one logger and that is enough.
