# Removing gocloud.dev

A plan for replacing `gocloud.dev/blob` with an astrogo-native storage layer
inside `remote/file`.

This document is the design. It is meant to be argued with before any of it is
built.

---

## 1. What it costs, measured

`gocloud.dev/blob` is 2,711 lines of astrogo's `remote/file` talking to a
library that brings 141 packages of other people's telemetry with it.

Counting linked packages — `go list -deps`, so what a consumer's binary
actually contains:

| import | total packages | gRPC | OpenTelemetry | gocloud | Arrow |
| :--- | ---: | ---: | ---: | ---: | ---: |
| `astrogo/time` | 93 | 0 | 0 | 0 | 0 |
| `astrogo/coord` | 100 | 0 | 0 | 0 | 0 |
| `astrogo/ephemeris` | 105 | 0 | 0 | 0 | 0 |
| **`astrogo/remote`** | **406** | **64** | **34** | **9** | 0 |
| **`astrogo/plan`** | **433** | **64** | **34** | **9** | 0 |

The scientific packages carry nothing. The moment a consumer reaches `remote` —
and `plan` does, for EOP and JPL kernels — the graph triples.

### 1.1 Where it comes from, and it is not the drivers

The obvious hypothesis is that the cloud drivers are the weight, and they are
already opt-in subpackages (`remote/file/s3` and friends export nothing at all,
they are blank imports). That hypothesis is wrong:

| package | total | heavy |
| :--- | ---: | ---: |
| `gocloud.dev/blob` alone | 388 | 141 |
| `+ gocloud.dev/blob/fileblob` | 390 | 141 |
| `+ gocloud-ext/blob/httpblob` | 392 | 141 |

The core `blob` package carries all of it; the two drivers astrogo links by
default add four packages between them. `blob.go` imports
`gocloud.dev/internal/otel`, which imports `go.opentelemetry.io/otel/sdk/metric`
unconditionally — that shim alone is 385 packages, 141 of them heavy.

So astrogo links the OpenTelemetry SDK, and gRPC and protobuf behind it, **so
that `gocloud.dev/blob` can emit traces and metrics nobody consumes.** Nothing
in astrogo configures an exporter, and nothing reads them.

### 1.2 There is no upstream escape

- v0.46.0 is the latest release; astrogo is already on a pseudo-version past it
  (pinned because `gocloud-ext`'s httpblob implements `driver.DeleteOptions`,
  added after that tag).
- The telemetry shim is not behind a build tag. `blob.go` imports it
  unconditionally.
- It is not `gocloud-ext`'s doing either — that was worth checking, since it is
  ours to fix. `httpblob` does import `googleapis/gax-go/v2`, but removing that
  would save two packages.

### 1.3 The second reason, which is not about weight

[#315](https://github.com/TuSKan/astrogo/issues/315) is open and unfixed: two
processes writing one cache key collide in `os.TempDir`, because `fileblob`
names its staging file from `time.Now().UnixNano()` and **the Windows clock does
not advance** — measured, one distinct value across 2000 consecutive reads. Both
writers pick the same path and one renames the other's file away.

astrogo has worked around this twice ([#241](https://github.com/TuSKan/astrogo/issues/241),
[#307](https://github.com/TuSKan/astrogo/pull/307)) with an in-process lock keyed
by basename, and `writelock.go`'s own doc comment states the boundary it cannot
reach: two processes. The staging strategy belongs to the driver, and while the
driver is someone else's, the fix is someone else's.

---

## 2. What astrogo actually uses

Small enough to write down completely. Every gocloud symbol in the tree:

```
blob.OpenBucket        blob.Bucket        blob.WriterOptions   blob.DefaultURLMux
gcerrors.Code          gcerrors.NotFound  gcerrors.Unknown     gcerrors.FailedPrecondition
driver.Bucket          driver.Writer
```

And every `Bucket` method called:

| method | why |
| :--- | :--- |
| `NewReader`, `NewRangeReader` | `remote.NewReaderAt`'s chunked LRU, SPK kernels |
| `NewWriter`, `WriteAll` | `remote.Save`, the download staging |
| `ReadAll` | small objects — sidecars, manifests |
| `Exists`, `Attributes` | cache hits, ETag checks for `Mutable` endpoints |
| `Delete`, `Copy` | staging promotion and cleanup |
| `Close` | |
| `As` | the `IfNotExist` staging lock reaches through to `driver.Writer` |

Eleven methods and four error codes. That is the contract to reproduce — not
gocloud's API, which is much larger, but the part astrogo depends on.

---

## 3. What the replacement must preserve

These are not implementation details; they are decisions CLAUDE.md records with
reasons, and a replacement that quietly drops one is a regression even if every
test passes.

- **No scheme is ever hardcoded in `remote` or `remote/file`.** Dispatch is a
  registry populated by blank imports. No `Backend`/`Transport` interface with a
  `switch` on scheme, no per-scheme branch in the fetch path.
- **`Endpoint.URL` is the exact string handed to the opener.** Scheme-specific
  connection detail rides in the URL query string — `remote.CopernicusEODATA`
  carries `region`, `endpoint`, `hostname_immutable` and `use_path_style` that
  way, which is why reaching a non-AWS S3 service needs no astrogo API and no
  AWS type in any signature.
- **Two portable query wrappers work on every scheme**: `?prefix=sub/dir/` scopes
  a bucket, `?key=exact/object.dat` serves one object under any name. Both come
  free from gocloud today and would have to be implemented once, centrally.
- **No API takes an OS filesystem path.** Bucket keys are `/`-separated;
  `path.Join`, never `filepath.Join`. The single OS-path contact point stays the
  unexported default-cache-dir resolver.
- **The opt-in subpackages export nothing.** `remote/file/s3` is four lines and
  zero exported symbols. That shape is the point and it must survive.
- **`remote.GetFile`'s semantics**: cached-on-existence for `Mutable: false`,
  ETag-checked for `Mutable: true`, download under a cross-process `IfNotExist`
  lock, `WithValidate` running against the *staged* object before promotion so a
  multi-GB kernel never has to fit in memory.
- **`remote.NewReaderAt`'s measured behaviour**: 64 KiB × 16 aligned chunks,
  1 MiB resident for any object. `BenchmarkReadAtStrategies` records why —
  263 ms against 0.33 ms for 2000 SPK-shaped reads — and must be re-run, not
  assumed.

---

## 4. The shape

**An astrogo-native driver interface inside `remote/file`, and adapters for the
schemes that need a cloud SDK.**

```go
// remote/file, unexported except where the front door re-exports it.
type driver interface {
    Reader(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)
    Writer(ctx context.Context, key string, opts writeOptions) (io.WriteCloser, error)
    Attributes(ctx context.Context, key string) (Attributes, error)
    Delete(ctx context.Context, key string) error
    Close() error
}
```

Five methods. `Exists` is `Attributes` with a `NotFound`; `ReadAll`/`WriteAll`
are helpers over the two streams; `Copy` is a read into a write, which is what
the cloud drivers do anyway for anything astrogo copies.

Three backends ship in core, and they are the ones with no SDK behind them:

| scheme | backend | what it is |
| :--- | :--- | :--- |
| `file://` | `os` + a staging rename | the cache, and astrogo's own default |
| `http://`, `https://` | `remote/api`'s client | range requests; already astrogo's own code in `gocloud-ext/httpblob` |
| `mem://` | a map | tests |

`s3`, `gcs`, `azure` and `sftp` become adapters in the same opt-in subpackages
they already live in, over each vendor's own SDK — which is where those SDKs
belong, since only a caller who blank-imports one pays for it. They are not
core's problem and never were; what made them core's problem is that
`gocloud.dev/blob` sits underneath all of them.

### 4.1 Why not a lighter third-party blob library

Because the surface in §2 is eleven methods, three of the backends have no SDK
at all, and the recurring lesson of the SGP4 work is that a dependency's *test
suite* is not evidence about the paths your code takes. The s⁴ constant was
wrong for years behind a suite that passed at full marks, and #315 is an
unfixable-by-us defect in a staging strategy we did not choose. Owning five
methods is less exposure than owning a seam to somebody else's five hundred.

The cloud adapters are the exception and stay third-party: reimplementing S3
request signing would be exactly the kind of thing this reasoning argues
against.

---

## 5. What this is expected to buy

Estimated, and to be measured rather than claimed:

- `astrogo/plan` from **433 linked packages to roughly 283** — a third of the
  graph.
- gRPC, protobuf and OpenTelemetry gone from every consumer that does not
  import `catalog/fink`, which pulls Arrow and its own gRPC independently. That
  distinction matters: **this refactor does not remove gRPC from a FINK user**,
  and saying so up front stops the claim being overstated later.
- `go.mod`'s 100 requires and the 334-module graph fall substantially.
- [#109](https://github.com/TuSKan/astrogo/issues/109) — astrogo inherits a
  patch-level `go` directive from `gocloud-ext`, forcing a toolchain download in
  every CI job — is resolved by the same change.
- #315 becomes fixable, because the staging strategy becomes astrogo's.

---

## 6. Delivery

Each step green and independently reviewable, branched from `main` once its
predecessor merges.

| PR | contents | how it is judged |
| :-- | :--- | :--- |
| **1** | This document. | Is the plan right? |
| **2** | The `driver` interface, the scheme registry, and the `file://` backend, alongside gocloud rather than replacing it. Staging under astrogo's control, with #315's two-process case as an explicit test. | The existing `remote/file` suite passes against the new backend behind a flag; #315's reproduction fails before and passes after. |
| **3** | `http`/`https` and `mem`. | The HTTP range behaviour matches `httpblob`'s, `BenchmarkReadAtStrategies` re-run and recorded. |
| **4** | Switch `remote/file` over; delete the gocloud path; `gocloud.dev` out of `go.mod`. | The package-count table in §1 re-measured. Every `remote` test unchanged. |
| **5** | `s3`, `gcs`, `azure`, `sftp` as SDK adapters. | Each still exports nothing; each still registers by blank import; the live tests for each still pass. |

PR 2 is deliberately the one that carries #315: the two-process staging
collision is the clearest evidence that this layer should be astrogo's, and
fixing it first means the rest of the work is measured against a defect already
closed rather than one still open.

---

## 7. Risks

**The cloud adapters are the real work.** Three core backends are
straightforward; four SDK adapters are not, and each has live tests against a
real service. PR 5 is sequenced last so that a problem there cannot block the
weight reduction, which is what the change is for.

**`As` reaches through to the driver.** The `IfNotExist` staging lock uses
gocloud's escape hatch to get at `driver.Writer`. An astrogo-native driver makes
that a normal method rather than a reach-through, which is simpler — but the
lock's semantics are subtle and #241/#307 are its history. It gets tests before
it gets a rewrite.

**Nothing here changes `remote`'s public API.** `Bucket`, `OpenBucket`, `Save`,
`GetFile`, `ReaderAt`, `IsNotFound` and the endpoint registry are the front door
and stay exactly as they are. If a step needs to change one, that is a signal
the design is wrong, not a licence.

---

## 8. Explicitly not in scope

- **Reimplementing S3/GCS/Azure request signing.** The adapters use each
  vendor's SDK.
- **A general-purpose blob abstraction for anyone else to use.** This is
  `remote/file`'s internals; the public surface is `remote`'s and does not grow.
- **Removing Apache Arrow.** It is `catalog`'s columnar cache, it earns its
  place, and it is a separate question from this one.
- **Telemetry of astrogo's own.** The point is not to replace gocloud's traces
  with ours. `logging` is the one logger and that is enough.
