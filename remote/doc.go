// Package remote centralizes every external network access astrogo makes:
// a registry of all endpoints the library can contact, an HTTP client with
// retry/backoff and consistent error handling, a consent-gated file
// downloader, and the configurable location where downloaded data is
// stored.
//
// # Endpoints
//
// Every remote service astrogo can reach is enumerated as an [EndpointID]
// — there are no hidden hosts. Inspect them with [Endpoints]; block one
// with [Disable]; point one at a mirror or proxy with [SetURL]; cut all
// network access with [SetOffline].
//
// # Downloads are never automatic
//
// astrogo never downloads a file without consent. Constructing a JPL
// ephemeris provider whose kernel is missing from the data directory fails
// with [ErrDownloadDenied] — stating the file, its size, and its source —
// until you either pre-seed the file or grant consent per endpoint:
//
//	// Allow planetary kernels up to 200 MB (de440/de442 ≈ 115 MB) and
//	// the ~5 KB leap-second kernel:
//	remote.EnableDownloads(remote.NAIFSPK, 200<<20)
//	remote.EnableDownloads(remote.NAIFLSK, 0) // 0 = no size limit
//
// Once enabled, downloads proceed silently up to the configured size
// limit. [SetPolicy] installs a custom consent callback for finer control.
//
// Request/response API endpoints (catalog resolvers, the light-pollution
// client) don't need download consent: the network call is the documented
// purpose of the method that triggers it. They are still subject to
// [Disable] and [SetOffline].
//
// # Data location
//
// All downloaded data (JPL SPK/LSK kernels, the IERS EOP cache) lives
// under one configurable base directory — default
// os.UserCacheDir()/astrogo — via [SetDataDir]/[SetDataDirPath]. The
// location is a github.com/ungerik/go-fs File, so a blob/bucket filesystem
// registered under its own URI scheme (s3://, gs://) can back it without
// any astrogo call-site changes.
//
// # Offline / air-gapped deployments
//
// Pre-seed the data directory with the files you need (kernel .bsp files,
// naif0012.tls, a finals2000A EOP file), then:
//
//	remote.SetOffline(true)
//
// Every downloader checks the filesystem before the network, so pre-seeded
// deployments never dial out. See the README section "Data downloads &
// offline usage" for the full table of files, sizes, and locations.
package remote
