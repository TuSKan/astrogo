// Package remote is astrogo's I/O boundary: a registry of every external
// endpoint the library can reach, the download policy every bulk download
// asks, and the cache all fetched data lands in.
//
// It owns policy, and it is the only package a caller imports. Moving bytes
// belongs to two subpackages internal to it, split by what is being
// addressed rather than by protocol:
//
//   - remote/file — byte-addressable resources with a stable identity, a
//     size, and range semantics. SPK kernels, IERS bulletins, catalog CSVs,
//     GeoTIFF bundles. A file on http is a file with an http backend, not
//     an API.
//   - remote/api — request/response services whose returned document
//     depends on the query. SIMBAD, VizieR, Gaia, MAST, CelesTrak, FINK,
//     JPL SBDB and Horizons.
//
// Neither is importable from outside remote/, and that is a test rather
// than a convention. Everything they do is here: [FS], [File], [OpenFS],
// [Open], [WriteFile], [RemoveFile] and [Schemes] for files, and the
// [Client.Get], [Client.GetJSON], [Client.PostForm] and [Client.PostJSON]
// request methods, with [HTTPError] and [RetryPolicy], for services. A
// missing object is errors.Is(err, fs.ErrNotExist), as anywhere else in Go.
// A caller reaching a subpackage directly would be going around the gate
// below, which is not a shortcut — it is a program that ignores
// [SetOffline] at one call site and reports nothing.
//
// # One policy, or one per component
//
// Everything below — offline mode, the download policy, endpoint overrides,
// the cache location — is a policy, and [Client] is that policy as a value. The
// package-level functions operate on [Default], the way net/http's
// package-level Get operates on http.DefaultClient, so a program with a single
// policy writes exactly what it always wrote. A binary whose components want
// different ones builds them:
//
//	prefetch := remote.NewClient() // downloads what it is asked for
//
//	serve := remote.NewClient()
//	serve.SetOffline(true)
//
// Every function documented below has a [Client] method of the same name.
//
// # Endpoints
//
// Every service astrogo can contact is an [EndpointID] — there are no
// hidden hosts. Inspect them with [Endpoints], block one with [Disable],
// point one at a mirror with [SetURL], cut all access with [SetOffline].
// [URL] is the single gate every call site passes through, so those three
// controls apply uniformly to files and APIs alike.
//
// # Downloads
//
// A call that needs a file downloads it the first time, and caches it: a JPL
// provider whose kernel is missing fetches it, of any size. Every download is
// logged at Warn, which the default logger shows, naming the endpoint, the
// object and its size; a cache hit logs nothing.
//
// [SetPolicy] decides otherwise. Every download asks the policy first, once
// against the endpoint's registered estimate before any request and again
// against the size the source actually reports, and a refusal is
// [ErrDownloadDenied] naming the file, its size and its source:
//
//	remote.SetPolicy(func(ep remote.Endpoint, size int64) error {
//		if size > 500<<20 {
//			return fmt.Errorf("%d bytes is over the 500 MB this program allows", size)
//		}
//
//		return nil
//	})
//
// An API call is not a download, and the policy is never asked about it — the
// request is the documented purpose of the method making it. The exception is
// [JPLHorizonsSPK], whose response carries a whole kernel; it is a separate
// endpoint from [JPLHorizons] so that the policy sees kernel generation
// without also seeing name resolution.
//
// # Where data lives
//
// Everything astrogo caches lives under one location, set by [SetDataDir]
// or the ASTROGO_CACHE_DIR environment variable, defaulting to
// os.UserCacheDir()/astrogo. It is a filesystem URL, not an OS path:
//
//	remote.SetDataDir("file:///data/astrogo?create_dir=true")
//	remote.SetDataDir("mem://scratch")       // for a test, or a cache that never lands
//
// Nothing in astrogo assumes the cache is local disk. [CacheDir] returns an
// [FS] and a key prefix; [GetFile] returns an [FS] and a key. There is no API
// anywhere that takes an OS path, and no local-only fast path — a cache on
// any scheme with a registered backend behaves identically.
//
// # Fetching
//
// [GetFile] is the one entry point for file content. It resolves the
// source and the cache through the same opener, reuses a cached copy (on
// existence alone for immutable content, after an ETag check for
// [Endpoint.Mutable] content when the caller may download), and on a
// miss downloads under a cross-process lock, staging the bytes and
// validating them before anything appears at the cache key. An
// interrupted transfer resumes from the partial rather than restarting.
//
// The policy is asked before any request. A caller whose policy refuses is
// served what is cached, mutable or not, as offline mode serves it, and
// refused only when nothing is: it could not download a newer copy, so asking
// the source whether there is one would only cost it.
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
//
// # Concurrency
//
// A [Client] is safe for concurrent use, and so are the package-level
// functions, which act on [Default]. A [File] from [Open] may be read with
// ReadAt from any number of goroutines at once; its Read and Seek move one
// offset, as an os.File's do.
package remote
