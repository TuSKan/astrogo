package remote

import "time"

// EndpointID names a remote service astrogo can contact. The full set of
// endpoints the library will ever reach is enumerated below — there are no
// hidden hosts. Inspect them at runtime with Endpoints.
type EndpointID string

// All remote services astrogo can contact.
const (
	// IERSFinals2000A is the IERS Earth-orientation-parameters file
	// (finals2000A.all, ~3.7 MB), used by the iers package for DUT1 and
	// polar motion.
	IERSFinals2000A EndpointID = "iers.finals2000A"

	// NAIFSPK is NASA NAIF's generic-kernels SPK directory, from which
	// ephemeris/jpl downloads planetary ephemeris kernels. Kernel sizes
	// vary widely: de440s ≈ 32 MB, de440/de442 ≈ 115 MB, de441 parts are
	// multi-GB.
	NAIFSPK EndpointID = "naif.spk"

	// NAIFLSK is NASA NAIF's generic-kernels directory for the leap-second
	// kernel (naif0012.tls, ~5 KB) required by ephemeris/jpl.
	NAIFLSK EndpointID = "naif.lsk"

	// JPLHorizons is the JPL Horizons API, used both for catalog/jpl name
	// resolution (small text responses) and by ephemeris/jpl to generate
	// small-body SPK kernels (KB–MB scale files).
	JPLHorizons EndpointID = "jpl.horizons"

	// JPLSBDB is the JPL Small-Body Database identify API (catalog/sbdb) —
	// resolves one object by name/designation.
	JPLSBDB EndpointID = "jpl.sbdb"

	// JPLSBDBQuery is JPL's separate SBDB *query* API (catalog/sbdb) — bulk
	// enumerates asteroids/comets matching a filter (e.g. absolute magnitude
	// below a bound), distinct from JPLSBDB's single-object identify
	// endpoint despite both living under the same JPL SBDB service.
	JPLSBDBQuery EndpointID = "jpl.sbdb.query"

	// SIMBAD is the CDS SIMBAD TAP service (catalog/simbad).
	SIMBAD EndpointID = "cds.simbad"

	// VizieR is the CDS VizieR TAP service (catalog/vizier).
	VizieR EndpointID = "cds.vizier"

	// GaiaTAP is ESA's Gaia archive TAP service (catalog/gaia).
	GaiaTAP EndpointID = "esa.gaia"

	// MAST is STScI's MAST invoke API (catalog/mast).
	MAST EndpointID = "stsci.mast"

	// CelesTrak is CelesTrak's GP element-set API (catalog/norad).
	CelesTrak EndpointID = "celestrak.gp"

	// FINK is the FINK broker's ZTF SSOFT API (catalog/fink).
	FINK EndpointID = "fink.ssoft"

	// LightPollution is the lightpollutionmap.info raster query API
	// (lightpollution package; requires an API key).
	LightPollution EndpointID = "lightpollutionmap"

	// OpenNGC is the OpenNGC catalog source CSVs on GitHub, pinned to a
	// specific commit so catalog/openngc.New's fetch is reproducible. Used
	// only when the caller has called remote.EnableDownloads(OpenNGC, ...)
	// — never implicitly.
	OpenNGC EndpointID = "openngc.github"

	// Nominatim is OpenStreetMap's Nominatim geocoding API, used by
	// plan.NewSiteEarthAddress to resolve a free-text address into
	// latitude/longitude.
	Nominatim EndpointID = "osm.nominatim"

	// OpenElevation is the Open-Elevation API, used by
	// plan.NewSiteEarthAddress to resolve a latitude/longitude (from
	// Nominatim) into a height above sea level.
	OpenElevation EndpointID = "open-elevation"

	// VIIRSAnnual is lightpollutionmap.info's own unauthenticated mirror of
	// NASA's VIIRS annual nighttime-lights composites (Black Marble
	// VNP46A4/VJ146A4, "AllAngle_Composite_Snow_Free" subset), one raw
	// single-band Float32 GeoTIFF zip per year 2012-2025
	// (viirs_<year>_raw.zip, base URL
	// https://www2.lightpollutionmap.info/data/v2/, caller supplies the
	// year-specific filename — the year varies per call the same way a
	// NAIFSPK kernel name does). Live-confirmed 2026-08-01: no login/API
	// key required, unlike NOAA/EOG's own hosting of the same underlying
	// product, which now requires OAuth2. Source data is CC0 (public
	// domain; NASA Black Marble). LICENSE NOTE for the mirror itself: per
	// lightpollutionmap.info/help.html FAQ 14, using VIIRS/Sky Brightness
	// data from this site "should be credited to 'Jurij Stare,
	// www.lightpollutionmap.info'" and should "also include 'NASA's Black
	// Marble nighttime lights product'". Used by
	// skybrightness/atlas.EnsureVIIRSAnnual/.OpenVIIRSAnnual.
	VIIRSAnnual EndpointID = "lightpollutionmap.viirs"

	// PassbandBundle is astrogo's own versioned, checksummed bundle of
	// photometric passband response curves (Johnson-Cousins UBVRI, Sloan
	// ugriz, Gaia G/BP/RP, CIE photopic/scotopic, SQM), used by
	// skybrightness/dataset/passband — the only place a passband curve
	// enters astrogo's runtime; core skybrightness never tabulates one in
	// Go source (docs/skybrightness.md §3). No bundle is published here
	// yet as of Sky Brightness V2 Phase 1 — see
	// skybrightness/dataset/passband's doc comment.
	PassbandBundle EndpointID = "astrogo.passbands"

	// WorldAtlas is GFZ Data Services' hosting of Falchi et al. 2016's
	// World Atlas 2015 of artificial night sky brightness (World_Atlas_2015.zip,
	// ~653 MB, frozen since 2019-11-18 under DOI 10.5880/GFZ.1.4.2016.001),
	// downloaded on demand by skybrightness/atlas.EnsureWorldAtlas.
	//
	// LICENSE: this dataset is CC BY-NC 4.0 (Attribution-NonCommercial) —
	// https://creativecommons.org/licenses/by-nc/4.0/, confirmed against
	// GFZ's own catalog record for this DOI. Non-commercial use only;
	// callers must attribute Falchi, C.C.M., et al. (2016), "The new world
	// atlas of artificial night sky brightness", Science Advances 2,
	// e1600377. This notice is deliberately repeated in
	// EnsureWorldAtlas's own doc comment and in the download consent log
	// line — not buried here alone.
	WorldAtlas EndpointID = "gfz.worldatlas"

	// CopernicusEODATA is the Copernicus Data Space Ecosystem's
	// S3-compatible "eodata" object-storage bucket
	// (https://eodata.dataspace.copernicus.eu). It is a general,
	// multi-product access point — Sentinel imagery, CLMS, and CAMS all
	// live in the same bucket, separated only by key prefix — so it is
	// named for the service, matching NAIFSPK's one-endpoint/many-
	// resources pattern rather than WorldAtlas's one-endpoint/one-dataset
	// pattern. CAMS-specific key construction belongs to
	// atmosphere/dataset/cams, not to this endpoint's identity.
	//
	// Access requires Copernicus Data Space credentials (free registration
	// at dataspace.copernicus.eu, then S3 keys generated on the user's own
	// dashboard). astrogo never reads a credential file of its own: the
	// remote/s3 transport resolves credentials exclusively through AWS SDK
	// v2's standard default chain (AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY
	// environment variables, ~/.aws/credentials, or an IAM role). The
	// service URL above is a fixed public fact, identical for every
	// Copernicus user, and is therefore registered here rather than read
	// from an AWS_ENDPOINT_URL environment variable.
	CopernicusEODATA EndpointID = "copernicus.eodata"
)

// Kind distinguishes request/response APIs from bulk file downloads.
type Kind string

const (
	// KindAPI marks request/response endpoints whose network access is the
	// explicit, documented purpose of the call that triggers it (a catalog
	// resolve, a light-pollution lookup). Enabled by default; disable
	// individually or via SetOffline.
	KindAPI Kind = "api"

	// KindFile marks bulk file-download endpoints. Downloads are DENIED by
	// default and must be enabled per endpoint with EnableDownloads.
	KindFile Kind = "file"

	// KindS3 marks bulk file endpoints served over the S3 object-storage
	// protocol rather than HTTP. Consent, caching, and GetFile semantics
	// are identical to KindFile; only the byte-transfer step differs, and
	// that step lives behind remote.Transport — remote itself never links
	// an S3 client. A KindS3 endpoint addresses objects by Bucket + key,
	// with URL naming the S3-compatible service endpoint. Import
	// remote/s3 and call its Register function before the first GetFile
	// against a KindS3 endpoint, or GetFile fails with ErrNoTransport.
	KindS3 Kind = "s3"
)

// cacheable reports whether endpoints of this Kind deliver bulk file
// content into remote's on-disk cache — i.e. whether CacheDir and GetFile
// apply to them. KindAPI endpoints are request/response only and have no
// cache directory.
func (k Kind) cacheable() bool { return k == KindFile || k == KindS3 }

// Endpoint describes one remote service: where it lives, what it is for,
// and how much data a request against it typically moves.
type Endpoint struct {
	// ID is the registry key.
	ID EndpointID
	// URL is the endpoint's base URL. Override with SetURL to point at a
	// mirror or proxy.
	URL string
	// Bucket is the S3 bucket name for a KindS3 endpoint, addressed
	// together with a caller-supplied object key (the name argument to
	// GetFile). Empty for every other Kind, where URL + path-join is the
	// whole address. Note the bucket is not implied by URL: one
	// S3-compatible service endpoint can host many buckets, and one
	// bucket (Copernicus's "eodata") hosts many unrelated products
	// distinguished only by key prefix.
	Bucket string
	// Kind is KindAPI, KindFile, or KindS3.
	Kind Kind
	// Subsystem names the astrogo package family using this endpoint. For
	// KindFile it is also the literal cache-dir token resolved by CacheDir;
	// for KindAPI it is a free-form description, never used for a path.
	Subsystem string
	// Description says what the endpoint provides.
	Description string
	// ApproxSize is the typical bytes moved per fetch; -1 means it varies
	// too much to state (NAIF SPK kernels range 5 KB–multi-GB).
	ApproxSize int64
	// Enabled gates all access to the endpoint (API calls and downloads).
	Enabled bool
	// DownloadsOK is the file-download consent flag. Always false by
	// default — see EnableDownloads.
	DownloadsOK bool
	// MaxDownloadSize caps a single download's size in bytes once
	// DownloadsOK is set; 0 means unlimited.
	MaxDownloadSize int64
	// Timeout is the API request timeout (KindAPI). Zero means
	// DefaultAPITimeout.
	Timeout time.Duration
	// DownloadTimeout is the whole-transfer timeout (KindFile). Zero means
	// DefaultDownloadTimeout.
	DownloadTimeout time.Duration
	// Mutable marks a KindFile endpoint whose upstream content can change
	// (IERS, OpenNGC): a cached copy is re-validated with a HEAD probe
	// before reuse. false means the content is immutable/versioned (JPL
	// kernels): a cached copy is reused on existence alone.
	Mutable bool
	// Files lists the fixed set of file names a KindFile endpoint serves,
	// for endpoints whose content is a small, known manifest rather than
	// arbitrarily-named caller-supplied files (e.g. OpenNGC's two source
	// CSVs). Empty for endpoints without a fixed manifest — JPL kernels are
	// named by the caller, so NAIFSPK/NAIFLSK leave this nil.
	Files []string
	// Downloadable reports whether this endpoint can deliver bulk file
	// content through the CheckDownload/GetFile consent gate — i.e.
	// whether EnableDownloads(id, ...) means anything for it. True for
	// every KindFile endpoint, and additionally for JPLHorizons, which is
	// a KindAPI endpoint that nonetheless returns a whole SPK kernel
	// (base64-encoded inside its JSON body) from
	// ephemeris/jpl/spk.CacheAPI's small-body ephemeris generation. A
	// KindAPI endpoint that only ever returns small text/JSON payloads
	// (SIMBAD, VizieR, SBDB, Gaia, MAST, CelesTrak, FINK, ...) leaves this
	// false: it has no download-consent gate at all, and
	// EnableAllDownloads/DisableAllDownloads deliberately never touch it.
	Downloadable bool
}

// SizeVaries marks an endpoint whose per-fetch size cannot be usefully
// approximated.
const SizeVaries int64 = -1

// DefaultDownloadTimeout is used for a KindFile endpoint whose
// DownloadTimeout is zero.
const DefaultDownloadTimeout = 10 * time.Minute

// defaultEndpoints is the built-in registry table. URLs here are the single
// source of truth for where astrogo connects — packages resolve them via
// URL(id) at request-build time.
func defaultEndpoints() map[EndpointID]Endpoint {
	return map[EndpointID]Endpoint{
		IERSFinals2000A: {
			ID:              IERSFinals2000A,
			URL:             "https://datacenter.iers.org/data/9/finals2000A.all",
			Kind:            KindFile,
			Subsystem:       "iers",
			Description:     "IERS Earth-orientation parameters (finals2000A.all)",
			ApproxSize:      3_800_000,
			Enabled:         true,
			DownloadTimeout: 30 * time.Second,
			Mutable:         true,
			Downloadable:    true,
		},
		NAIFSPK: {
			ID:              NAIFSPK,
			URL:             "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/spk/",
			Kind:            KindFile,
			Subsystem:       "jpl",
			Description:     "NASA NAIF planetary ephemeris SPK kernels (de440s ~32 MB, de440/de442 ~115 MB, de441 multi-GB)",
			ApproxSize:      SizeVaries,
			Enabled:         true,
			DownloadTimeout: 30 * time.Minute,
			Mutable:         false,
			Downloadable:    true,
		},
		NAIFLSK: {
			ID:              NAIFLSK,
			URL:             "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/",
			Kind:            KindFile,
			Subsystem:       "jpl",
			Description:     "NASA NAIF leap-second kernel (naif0012.tls)",
			ApproxSize:      6_000,
			Enabled:         true,
			DownloadTimeout: 1 * time.Minute,
			Mutable:         false,
			Downloadable:    true,
		},
		JPLHorizons: {
			ID:           JPLHorizons,
			URL:          "https://ssd.jpl.nasa.gov/api/horizons.api",
			Kind:         KindAPI,
			Subsystem:    "ephemeris/jpl, catalog/jpl",
			Description:  "JPL Horizons API (name resolution and small-body SPK generation)",
			ApproxSize:   SizeVaries,
			Enabled:      true,
			Timeout:      2 * time.Minute,
			Downloadable: true,
		},
		JPLSBDB: {
			ID:          JPLSBDB,
			URL:         "https://ssd-api.jpl.nasa.gov/sbdb.api",
			Kind:        KindAPI,
			Subsystem:   "catalog/sbdb",
			Description: "JPL Small-Body Database identify API",
			ApproxSize:  100_000,
			Enabled:     true,
			Timeout:     30 * time.Second,
		},
		JPLSBDBQuery: {
			ID:          JPLSBDBQuery,
			URL:         "https://ssd-api.jpl.nasa.gov/sbdb_query.api",
			Kind:        KindAPI,
			Subsystem:   "catalog/sbdb",
			Description: "JPL Small-Body Database bulk query API (asteroid/comet browse)",
			ApproxSize:  SizeVaries,
			Enabled:     true,
			Timeout:     30 * time.Second,
		},
		SIMBAD: {
			ID:          SIMBAD,
			URL:         "http://simbad.cds.unistra.fr/simbad/sim-tap/sync",
			Kind:        KindAPI,
			Subsystem:   "catalog/simbad",
			Description: "CDS SIMBAD TAP service",
			ApproxSize:  100_000,
			Enabled:     true,
			Timeout:     30 * time.Second,
		},
		VizieR: {
			ID:          VizieR,
			URL:         "http://tapvizier.u-strasbg.fr/TAPVizieR/tap/sync",
			Kind:        KindAPI,
			Subsystem:   "catalog/vizier",
			Description: "CDS VizieR TAP service",
			ApproxSize:  100_000,
			Enabled:     true,
			Timeout:     30 * time.Second,
		},
		GaiaTAP: {
			ID:          GaiaTAP,
			URL:         "https://gea.esac.esa.int/tap-server/tap/sync",
			Kind:        KindAPI,
			Subsystem:   "catalog/gaia",
			Description: "ESA Gaia archive TAP service",
			ApproxSize:  100_000,
			Enabled:     true,
			Timeout:     30 * time.Second,
		},
		MAST: {
			ID:          MAST,
			URL:         "https://mast.stsci.edu/api/v0/invoke",
			Kind:        KindAPI,
			Subsystem:   "catalog/mast",
			Description: "STScI MAST invoke API",
			ApproxSize:  100_000,
			Enabled:     true,
			Timeout:     30 * time.Second,
		},
		CelesTrak: {
			ID:          CelesTrak,
			URL:         "https://celestrak.org/NORAD/elements/gp.php",
			Kind:        KindAPI,
			Subsystem:   "catalog/norad",
			Description: "CelesTrak GP element sets (TLEs)",
			ApproxSize:  100_000,
			Enabled:     true,
			Timeout:     30 * time.Second,
		},
		FINK: {
			ID:          FINK,
			URL:         "https://api.ztf.fink-portal.org/api/v1/ssoft",
			Kind:        KindAPI,
			Subsystem:   "catalog/fink",
			Description: "FINK broker ZTF solar-system object feature table",
			ApproxSize:  SizeVaries,
			Enabled:     true,
			Timeout:     120 * time.Second,
		},
		LightPollution: {
			ID:          LightPollution,
			URL:         "https://www.lightpollutionmap.info/QueryRaster/",
			Kind:        KindAPI,
			Subsystem:   "lightpollution",
			Description: "lightpollutionmap.info raster query (World Atlas 2015)",
			ApproxSize:  1_000,
			Enabled:     true,
			Timeout:     30 * time.Second,
		},
		OpenNGC: {
			ID:              OpenNGC,
			URL:             "https://raw.githubusercontent.com/mattiaverga/OpenNGC/36cb178a0f69dba8bfc03a99c10512831edf1c6b/database_files",
			Kind:            KindFile,
			Subsystem:       "openngc",
			Description:     "OpenNGC catalog source CSVs (NGC.csv, addendum.csv), pinned to a fixed commit",
			ApproxSize:      2_000_000,
			Enabled:         true,
			DownloadTimeout: 2 * time.Minute,
			Mutable:         true,
			Downloadable:    true,
			Files:           []string{"NGC.csv", "addendum.csv"},
		},
		Nominatim: {
			ID:          Nominatim,
			URL:         "https://nominatim.openstreetmap.org/search",
			Kind:        KindAPI,
			Subsystem:   "plan",
			Description: "OpenStreetMap Nominatim geocoding API (address to coordinates)",
			ApproxSize:  2_000,
			Enabled:     true,
			Timeout:     30 * time.Second,
		},
		OpenElevation: {
			ID:          OpenElevation,
			URL:         "https://api.open-elevation.com/api/v1/lookup",
			Kind:        KindAPI,
			Subsystem:   "plan",
			Description: "Open-Elevation API (coordinates to height above sea level)",
			ApproxSize:  2_000,
			Enabled:     true,
			Timeout:     30 * time.Second,
		},
		VIIRSAnnual: {
			ID:   VIIRSAnnual,
			URL:  "https://www2.lightpollutionmap.info/data/v2/",
			Kind: KindFile,
			// Subsystem doubles as CacheDir's path token, same convention
			// as WorldAtlas below — both live under skybrightness/atlas's
			// cache directory since both feed the same GeoTIFF decoder.
			Subsystem: "atlas",
			Description: "NASA VIIRS annual nighttime-lights composites (Black Marble " +
				"VNP46A4/VJ146A4), mirrored unauthenticated by lightpollutionmap.info, " +
				"one raw GeoTIFF zip per year 2012-2025. Source data is CC0; the mirror " +
				"itself asks for credit to \"Jurij Stare, www.lightpollutionmap.info\" " +
				"plus \"NASA's Black Marble nighttime lights product\" " +
				"(lightpollutionmap.info/help.html FAQ 14).",
			ApproxSize:      SizeVaries, // ~700 MB (2012) to ~1 GB (2025), grows over time
			Enabled:         true,
			DownloadTimeout: 60 * time.Minute,
			Mutable:         true, // past years are occasionally reprocessed in place (e.g. the 2025-06 Black Marble v2.0 switch)
			Downloadable:    true,
		},
		PassbandBundle: {
			ID:        PassbandBundle,
			URL:       "https://github.com/TuSKan/astrogo/releases/download/passbands-v1/",
			Kind:      KindFile,
			Subsystem: "skybrightness/dataset/passband",
			Description: "Versioned, checksummed passband response-curve bundle " +
				"(Johnson-Cousins, Sloan, Gaia, CIE, SQM) — not yet published as of " +
				"Sky Brightness V2 Phase 1; endpoint declared ahead of the dataset " +
				"per this registry's own convention (see WorldAtlas/VIIRSAnnual).",
			ApproxSize:      2 << 20,
			Enabled:         true,
			DownloadTimeout: 2 * time.Minute,
			Mutable:         false, // pinned by semver + checksums in the manifest
			Downloadable:    true,
			Files:           []string{"passbands-v1.tar.gz"},
		},
		WorldAtlas: {
			ID:   WorldAtlas,
			URL:  "https://datapub.gfz.de/download/10.5880.GFZ.1.4.2016.001/",
			Kind: KindFile,
			// Subsystem doubles as CacheDir's path token (see Endpoint.Subsystem) —
			// "atlas" for skybrightness/atlas's other offline decoders. Nested
			// tokens are supported (see PassbandBundle above) and resolve to
			// nested directories on every platform via subsystemDir/filepath.Join.
			Subsystem: "atlas",
			Description: "Falchi et al. 2016 World Atlas 2015 of artificial night sky " +
				"brightness (GFZ Data Services, DOI 10.5880/GFZ.1.4.2016.001, frozen " +
				"2019-11-18). LICENSE: CC BY-NC 4.0 (non-commercial), " +
				"https://creativecommons.org/licenses/by-nc/4.0/ — attribute Falchi " +
				"et al. (2016), Sci. Adv. 2, e1600377.",
			ApproxSize:      684_266_450, // World_Atlas_2015.zip, live-confirmed Content-Length
			Enabled:         true,
			DownloadTimeout: 60 * time.Minute,
			Mutable:         false, // DOI-versioned, frozen since 2019-11-18
			Downloadable:    true,
			Files:           []string{"World_Atlas_2015.zip"},
		},
		CopernicusEODATA: {
			ID:        CopernicusEODATA,
			URL:       "https://eodata.dataspace.copernicus.eu",
			Bucket:    "eodata",
			Kind:      KindS3,
			Subsystem: "atmosphere/dataset/cams",
			Description: "Copernicus Data Space Ecosystem EODATA S3 bucket — a general " +
				"multi-product object store (Sentinel, CLMS, CAMS, ...) keyed by product " +
				"prefix, not a CAMS-specific service. astrogo uses the CAMS/GLOBAL/... " +
				"prefix for CAMS global analysis NetCDF-4 files (one variable per file, " +
				"1.3 MB for lnsp up to ~180 MB for a 137-level aerosol tracer, " +
				"live-measured). Requires Copernicus Data Space S3 credentials supplied " +
				"through the standard AWS SDK v2 credential chain; astrogo reads no " +
				"credential file of its own. NOTE: unlike astrogo's HTTP transport, this " +
				"one has no Range-based resume.",
			ApproxSize:      SizeVaries, // 1.3 MB (lnsp) to ~180 MB (a 137-level aerosol tracer)
			Enabled:         true,
			DownloadTimeout: 30 * time.Minute,
			Mutable:         true, // CAMS publishes new analysis cycles ~4x/day and reprocesses
			Downloadable:    true,
		},
	}
}
