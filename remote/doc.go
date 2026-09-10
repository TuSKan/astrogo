// Package remote is astrogo's I/O boundary: a registry of every external
// endpoint the library can reach, a consent gate no bulk download bypasses,
// and the cache all fetched data lands in.
//
// It owns policy. Moving bytes belongs to its two subpackages, split by
// what is being addressed rather than by protocol:
//
//   - [github.com/TuSKan/astrogo/remote/file] — byte-addressable resources
//     with a stable identity, a size, and range semantics. SPK kernels,
//     IERS bulletins, catalog CSVs, GeoTIFF bundles. A file on http is a
//     file with an http backend, not an API.
//   - [github.com/TuSKan/astrogo/remote/api] — request/response services
//     whose returned document depends on the query. SIMBAD, VizieR, Gaia,
//     MAST, CelesTrak, FINK, JPL SBDB and Horizons.
//
// # One policy, or several
//
// Every function in this package operates on [Default], exactly as
// [net/http.Get] operates on [net/http.DefaultClient]. A program with one
// policy configures it with [EnableDownloads], [SetOffline] and [SetDataDir]
// and never mentions a [Client].
//
// A program with two builds them. An HTTP handler that must never block on a
// download and a background prefetcher whose whole job is to download are one
// binary with two policies, and each gets its own:
//
//	serving  := remote.NewClient(remote.WithOffline(true))
//	prefetch := remote.NewClient(remote.WithDownloads(2<<30, remote.NAIFSPK))
//
//	handler := jpl.NewProvider(ctx, core.Planets, "de440", jpl.WithClient(serving))
//	warmer  := jpl.NewProvider(ctx, core.Planets, "de440", jpl.WithClient(prefetch))
//
// Each consuming package takes the client as an option — [jpl.WithClient],
// [resolve.WithClient] for the catalog providers, [cams.WithClient],
// [api.WithRemote]. Omit it and you get [Default].
//
// One thing this does not reach: the Earth-orientation data [time] loads
// lazily. [time.Time.EOP] has no context and no error return by design, so
// there is nowhere for a per-component policy to enter, and that fetch always
// uses [Default]. A caller who needs different behaviour there registers their
// own model with time.RegisterModel.
//
// # Endpoints
//
// Every service astrogo can contact is an [EndpointID] — there are no
// hidden hosts. Inspect them with [Endpoints], block one with [Disable],
// point one at a mirror with [SetURL], cut all access with [SetOffline].
// [URL] is the single gate every call site passes through, so those three
// controls apply uniformly to files and APIs alike.
//
// # Downloads are never automatic
//
// astrogo never fetches a bulk file without consent. Constructing a JPL
// provider whose kernel is missing fails with [ErrDownloadDenied], naming
// the file, its size and its source, until you pre-seed it or grant
// consent:
//
//	// Planetary kernels up to 200 MB, plus the ~5 KB leap-second kernel:
//	remote.EnableDownloads(200<<20, remote.NAIFSPK, remote.NAIFLSK)
//
//	// Or everything that can download, with one cap:
//	remote.EnableDownloads(0)
//
// Consent is checked twice per fetch: once against the endpoint's
// registered estimate before any request, then again against the size the
// source actually reports. [SetPolicy] replaces both with your own rule.
//
// An API call needs no consent — the request is the documented purpose of
// the method making it. The exception is [JPLHorizonsSPK], whose response
// carries a whole kernel; it is a separate endpoint from [JPLHorizons] so
// that consent gates kernel generation without also gating name
// resolution.
//
// # Where data lives
//
// Everything astrogo caches lives under one location, set by [SetDataDir]
// or the ASTROGO_CACHE_DIR environment variable, defaulting to
// os.UserCacheDir()/astrogo. It is a bucket URL, not a filesystem path:
//
//	remote.SetDataDir("file:///data/astrogo?create_dir=true")
//	remote.SetDataDir("s3://my-cache-bucket") // needs a blank import of remote/s3
//
// Nothing in astrogo assumes the cache is local disk. [CacheDir] returns a
// bucket and a key prefix; [GetFile] returns a bucket and a key. There is
// no API anywhere that takes an OS path, and no local-only fast path — a
// deployment whose cache is object storage behaves identically.
//
// # Fetching
//
// [GetFile] is the one entry point for file content. It resolves the
// source and the cache through the same opener, reuses a cached copy (on
// existence alone for immutable content, after an ETag check for
// [Endpoint.Mutable] content), and on a miss downloads under a
// cross-process lock, staging the bytes and validating them before
// anything appears at the cache key. An interrupted transfer resumes from
// the partial rather than restarting.
//
// # Offline and air-gapped deployments
//
// Pre-seed the cache with the objects you need, then:
//
//	remote.SetOffline(true)
//
// Every fetch checks the cache before the network, so a pre-seeded
// deployment never dials out even without SetOffline. See the README's
// "Data downloads & offline usage" for the full table of files, sizes and
// cache keys.
package remote
