// Package s3 is remote's S3-backed remote.Transport implementation for
// remote.KindS3 endpoints — currently just remote.CopernicusEODATA, the
// Copernicus Data Space Ecosystem's "eodata" bucket, used by
// atmosphere/dataset/cams to fetch CAMS global reanalysis NetCDF-4 files.
//
// # Why this is a separate package
//
// remote itself is documented (CLAUDE.md) as "primitives layer — stdlib +
// cenkalti/backoff/v5 + ungerik/go-fs only," and every package in this
// module transitively depends on remote. Importing the AWS SDK v2 (and
// github.com/ungerik/go-fs/s3fs, which wraps it) directly into remote
// would drag that dependency tree into every consumer's build graph, not
// just the one caller who actually needs S3. So remote itself gains only
// a small, dependency-free registration point (remote.Transport,
// remote.RegisterTransport); this package is the only importer of both
// s3fs and the AWS SDK, matching the same isolation this codebase already
// applies to github.com/scigolib/hdf5 (scoped to "the only importer
// within skybrightness/dataset/granule", not linked into the shared
// core). A build that never imports this package never links an S3
// client, full stop.
//
// # Registration is explicit, never init()
//
// Call Register once, before the first remote.GetFile call against a
// KindS3 endpoint; without it, GetFile fails with remote.ErrNoTransport.
// There is no package-level init() that registers anything automatically
// — CLAUDE.md's "no hidden global mutation or init() side effects" rule
// applies here as much as anywhere else in this codebase.
//
// # Credentials
//
// Register resolves credentials exclusively through the AWS SDK v2's
// standard default chain (environment variables, ~/.aws/credentials, or
// an IAM role) via config.LoadDefaultConfig. This package never reads a
// credential file of its own, never defines an astrogo-specific
// credential environment variable, and never reads AWS_ENDPOINT_URL —
// the service endpoint comes from the caller's own remote.Endpoint
// registry entry (remote.URL(id)), which is astrogo's single source of
// truth for where it connects and already carries the offline-mode/
// Disable/SetURL override gates every other endpoint gets.
//
// # Why FetchInto/Probe bypass s3fs's own File-reading methods
//
// Register still calls s3fs.NewAndRegister so that gofs.File("s3://" +
// bucket + "/...") becomes a working value through go-fs's own registry
// for any *other* future caller in this codebase who wants small-object
// convenience reads (accepting s3fs's current buffering limitation,
// below). But this package's own FetchInto/Probe — the two operations
// that must be both correct and memory-bounded for potentially large
// objects (a CAMS aerosol tracer file runs ~180 MB) — go straight to the
// same already-authenticated *s3.Client s3fs itself wraps, using the AWS
// SDK's own GetObject/HeadObject calls directly. This is not "hand-
// rolling S3 protocol details" — no SigV4 signing or XML response
// parsing is reimplemented here, only the SDK's own documented low-level
// calls — it is the same "use the SDK's own streaming primitive, not a
// reinvented wire protocol" pattern remote's built-in HTTP transport
// already uses for net/http. Two confirmed, real reasons this package
// does not route bytes through s3fs.ReadAll/OpenReader:
//
//  1. Wrong key. go-fs's File.ReadAll/OpenReader/Stat dispatch via
//     ParseRawURI, which for s3fs strips only the "s3://bucket" prefix —
//     gofs.File("s3://eodata/CAMS/GLOBAL/...") therefore yields the path
//     "/CAMS/GLOBAL/..." (a leading slash), and s3fs.go's ReadAll passes
//     that string verbatim as the S3 object Key with no trimming
//     anywhere in the function. Real EODATA object keys have no leading
//     slash, so a request built from a gofs.File URI would address the
//     wrong key and 404 against the real bucket — confirmed by reading
//     both go-fs's registry.go (ParseRawURI/CleanPathFromURI) and
//     s3fs.go's ReadAll directly, not assumed. s3fs's own tests never
//     catch this because they write and read back using the same
//     (consistently wrong) convention.
//  2. Whole-object-in-memory, always. Both s3fs.ReadAll and
//     s3fs.OpenReader buffer the entire object regardless of entry
//     point — plain io.ReadAll below 10 MB, a full in-memory
//     manager.WriteAtBuffer (not a streaming sink) above it. There is no
//     byte-range or true-streaming read anywhere in that package
//     version. Peak RSS would scale with object size (~180 MB for a
//     CAMS aerosol tracer, and this package makes no assumption that's
//     the largest object it will ever see) if routed through s3fs's own
//     File API.
//
// FetchInto instead streams the SDK's GetObjectOutput.Body directly to
// the destination via io.Copy, matching how remote's built-in HTTP
// transport already streams an http.Response.Body — peak memory stays
// bounded by the copy buffer, not the object size, in the common
// (non-validated) case; the validated case still buffers, matching the
// same tradeoff the HTTP transport already accepts for WithValidate.
//
// # What this transport does not offer, compared to remote's HTTP transport
//
// No Range-based resume for an interrupted transfer (the AWS SDK's
// GetObject does support a Range header, but wiring resume through it is
// not built here yet — see the design notes in docs/skybrightness.md's
// CAMS section for why byte-range reads against the object's internal
// format are a separate, larger, deliberately deferred piece of work,
// not a small addition to this transport). Progress reporting is
// incremental (wrapping the streamed Body, not "all at once" the way a
// buffered read would force), unlike the resume gap.
package s3
