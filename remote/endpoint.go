package remote

import "time"

// EndpointID names a remote service astrogo can contact. The set below is
// exhaustive — there are no hidden hosts. Inspect them at runtime with
// Endpoints.
type EndpointID string

// All remote services astrogo can contact.
const (
	// IERSFinals2000A is the IERS Earth-orientation-parameters directory
	// serving finals2000A.all (~3.7 MB), used by time for DUT1 and polar
	// motion.
	IERSFinals2000A EndpointID = "iers.finals2000A"

	// NAIFSPK is NASA NAIF's generic-kernels SPK directory, from which
	// ephemeris/jpl downloads planetary ephemeris kernels. Sizes vary
	// widely: de440s ~32 MB, de440/de442 ~115 MB, de441 parts multi-GB.
	NAIFSPK EndpointID = "naif.spk"

	// NAIFLSK is NASA NAIF's generic-kernels directory for the leap-second
	// kernel (naif0012.tls, ~5 KB) required by ephemeris/jpl.
	NAIFLSK EndpointID = "naif.lsk"

	// JPLHorizons is the JPL Horizons API used for small-body name
	// resolution (catalog/jpl) — small text responses only. Kernel
	// generation is a separate endpoint; see JPLHorizonsSPK.
	JPLHorizons EndpointID = "jpl.horizons"

	// JPLHorizonsSPK is the same JPL Horizons service used to generate a
	// small-body SPK kernel, which arrives base64-encoded inside the JSON
	// response (KB-MB). It is registered separately from JPLHorizons so
	// EnableDownloads gates kernel generation without also gating name
	// resolution, which needs no consent.
	JPLHorizonsSPK EndpointID = "jpl.horizons.spk"

	// JPLSBDB is the JPL Small-Body Database identify API (catalog/sbdb) —
	// resolves one object by name/designation.
	JPLSBDB EndpointID = "jpl.sbdb"

	// JPLSBDBQuery is JPL's separate SBDB query API (catalog/sbdb): bulk
	// enumeration of asteroids/comets matching a filter, distinct from
	// JPLSBDB's single-object identify endpoint.
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
	// (requires an API key).
	LightPollution EndpointID = "lightpollutionmap"

	// OpenNGC is the OpenNGC catalog source CSVs on GitHub, pinned to a
	// fixed commit so catalog/openngc's fetch is reproducible.
	OpenNGC EndpointID = "openngc.github"

	// Nominatim is OpenStreetMap's Nominatim geocoding API, used by
	// plan.NewSiteEarthAddress to resolve an address to coordinates.
	Nominatim EndpointID = "osm.nominatim"

	// OpenElevation is the Open-Elevation API, used by
	// plan.NewSiteEarthAddress to resolve coordinates to a height above
	// sea level.
	OpenElevation EndpointID = "open-elevation"

	// VIIRSAnnual is lightpollutionmap.info's unauthenticated mirror of
	// NASA's VIIRS annual nighttime-lights composites (Black Marble
	// VNP46A4/VJ146A4, "AllAngle_Composite_Snow_Free"), one raw
	// single-band Float32 GeoTIFF zip per year (viirs_<year>_raw.zip); the
	// caller supplies the year-specific name. Unlike NOAA/EOG's own
	// hosting of the same product, this mirror needs no OAuth2.
	//
	// LICENSE: the source data is CC0 (NASA Black Marble). The mirror asks
	// (lightpollutionmap.info/help.html FAQ 14) that use be credited to
	// "Jurij Stare, www.lightpollutionmap.info" and to "NASA's Black
	// Marble nighttime lights product".
	VIIRSAnnual EndpointID = "lightpollutionmap.viirs"

	// PassbandBundle is astrogo's own versioned, checksummed bundle of
	// photometric passband response curves (Johnson-Cousins UBVRI, Sloan
	// ugriz, Gaia G/BP/RP, CIE photopic/scotopic, SQM) for
	// skybrightness/dataset/passband — the only way a passband curve
	// enters astrogo's runtime (docs/skybrightness.md §3). No bundle is
	// published at this URL yet.
	PassbandBundle EndpointID = "astrogo.passbands"

	// WorldAtlas is GFZ Data Services' hosting of Falchi et al. 2016's
	// World Atlas 2015 of artificial night sky brightness
	// (World_Atlas_2015.zip, ~653 MB, frozen 2019-11-18 under DOI
	// 10.5880/GFZ.1.4.2016.001).
	//
	// LICENSE: CC BY-NC 4.0, non-commercial only. Attribute Falchi, C.C.M.,
	// et al. (2016), "The new world atlas of artificial night sky
	// brightness", Science Advances 2, e1600377.
	WorldAtlas EndpointID = "gfz.worldatlas"

	// CALSPEC is STScI's composite stellar and solar flux standards, the
	// absolute-flux reference the HST calibration chain is built on. The
	// solar spectrum here is what fixes the absolute scale of every
	// reflected-sunlight model in this library — lunar irradiance and
	// zodiacal light both — so which reference is used is a decision, not
	// a detail.
	CALSPEC EndpointID = "stsci.calspec"

	// GaiaStarMap is the prebuilt integrated-starlight map published with
	// astrogo's own releases (skybrightness/dataset/starlight).
	//
	// It is a release asset rather than a third-party archive because the map
	// is a derived product: 787 aggregation queries against the Gaia archive,
	// reduced to one 5 MB file. Publishing it means a caller never has to run
	// that against a shared service, and every caller gets the same numbers.
	GaiaStarMap EndpointID = "astrogo.starmap"

	// CopernicusEODATA is the Copernicus Data Space Ecosystem's
	// S3-compatible "eodata" object store. It is a general, multi-product
	// access point — Sentinel, CLMS and CAMS share one bucket, separated
	// only by key prefix — so it is named for the service, matching
	// NAIFSPK's one-endpoint/many-resources shape. CAMS-specific key
	// construction belongs to atmosphere/dataset/cams.
	//
	// Access needs Copernicus Data Space credentials (free registration,
	// then S3 keys from the user's own dashboard), resolved through AWS
	// SDK v2's standard chain. astrogo reads no credential file of its
	// own. A build using this endpoint must blank-import remote/s3.
	CopernicusEODATA EndpointID = "copernicus.eodata"
)

// Kind distinguishes request/response APIs from bulk file downloads. What
// backend serves a KindFile endpoint's bytes is a property of its URL's
// scheme, resolved by remote/file, not of Kind.
type Kind string

const (
	// KindAPI marks endpoints whose network access is the explicit purpose
	// of the call that triggers it (a catalog resolve, a raster lookup).
	// Reachable by default; gate with Disable or SetOffline.
	KindAPI Kind = "api"

	// KindFile marks bulk file endpoints. Downloads are denied by default
	// and need EnableDownloads.
	KindFile Kind = "file"
)

// cacheable reports whether this Kind delivers bulk file content into the
// cache — i.e. whether CacheDir and GetFile apply.
func (k Kind) cacheable() bool { return k == KindFile }

// Endpoint describes one remote service: where it lives, what it is for,
// and how much data a request against it moves.
type Endpoint struct {
	// ID is the registry key.
	ID EndpointID
	// URL is what the endpoint is addressed by. For KindFile it is the
	// exact string handed to blob.OpenBucket, and it must be a
	// directory-style prefix — the bucket root the caller's name argument
	// resolves within, never one exact resource. For KindAPI it is the
	// base request URL.
	//
	// Override with SetURL to reach a mirror, a proxy, or a different
	// bucket entirely. Because the string is passed through untouched,
	// everything blob.OpenBucket understands works here, on every scheme:
	// scheme-specific connection details the driver parses, plus gocloud's
	// own portable wrappers —
	//
	//	?prefix=mirror/openngc/   scope the bucket to a subdirectory
	//	?key=archive/dump.dat     serve one exact object under any name
	//
	// The "key" form is how to point an endpoint at a single file, since a
	// bare single-object URL has no room for the caller's name and simply
	// fails to resolve. See TestSetURLToSingleObjectViaKeyParam.
	URL string
	// Kind is KindAPI or KindFile.
	Kind Kind
	// Subsystem names the astrogo package family using this endpoint. For
	// KindFile it is also the cache key prefix resolved by CacheDir; for
	// KindAPI it is descriptive only.
	Subsystem string
	// Description says what the endpoint provides, including the real
	// service host when URL does not name it directly.
	Description string
	// ApproxSize is the typical bytes moved per fetch; SizeVaries means it
	// varies too much to state.
	ApproxSize int64
	// Enabled gates all access to the endpoint.
	Enabled bool
	// DownloadsOK is the file-download consent flag, false by default.
	// See EnableDownloads.
	DownloadsOK bool
	// MaxDownloadSize caps a single download in bytes once DownloadsOK is
	// set; 0 means unlimited.
	MaxDownloadSize int64
	// Timeout is the API request timeout (KindAPI). Zero means
	// DefaultAPITimeout.
	Timeout time.Duration
	// DownloadTimeout is the whole-transfer timeout (KindFile). Zero means
	// DefaultDownloadTimeout.
	DownloadTimeout time.Duration
	// Mutable marks a KindFile endpoint whose upstream content can change
	// (IERS, OpenNGC): a cached copy is revalidated against the source's
	// current ETag before reuse. false means versioned/immutable content
	// (JPL kernels), reused on existence alone.
	Mutable bool
	// Files lists the fixed set of names a KindFile endpoint serves, when
	// its content is a known manifest rather than caller-named objects.
	// Nil where the caller names the object (JPL kernels).
	Files []string
	// Downloadable reports whether EnableDownloads means anything for this
	// endpoint — true for every KindFile endpoint, and for JPLHorizonsSPK,
	// whose KindAPI response carries a whole kernel. A KindAPI endpoint
	// returning only small payloads leaves this false and has no consent
	// gate at all.
	Downloadable bool
}

// SizeVaries marks an endpoint whose per-fetch size cannot be usefully
// approximated.
const SizeVaries int64 = -1

// DefaultDownloadTimeout applies to a KindFile endpoint whose
// DownloadTimeout is zero.
const DefaultDownloadTimeout = 10 * time.Minute

// horizonsURL is shared by JPLHorizons and JPLHorizonsSPK: one service,
// two consent scopes.
const horizonsURL = "https://ssd.jpl.nasa.gov/api/horizons.api"

// copernicusEODATAURL addresses the Copernicus eodata bucket. Everything
// past "?" is s3blob URL-opener configuration (url.Values-encoded, parsed
// by gocloud.dev/blob/s3blob and gocloud.dev/aws): the service is not AWS,
// so it needs its real HTTPS host, an immutable hostname, path-style
// addressing, and a placeholder region the SDK will not reject. Expressing
// it here rather than in Go keeps remote free of S3-specific code and lets
// SetURL retarget it. See TestCopernicusURLCarriesS3ConnectionParams.
const copernicusEODATAURL = "s3://eodata?endpoint=https%3A%2F%2Feodata.dataspace.copernicus.eu" +
	"&hostname_immutable=true&region=default&use_path_style=true"

// defaultEndpoints is the built-in registry table — the single source of
// truth for where astrogo connects. Packages resolve a URL through URL(id)
// at request-build time, never from a literal of their own.
func defaultEndpoints() map[EndpointID]Endpoint {
	return map[EndpointID]Endpoint{
		IERSFinals2000A: {
			ID:              IERSFinals2000A,
			URL:             "https://datacenter.iers.org/data/9/",
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
			ID:          JPLHorizons,
			URL:         horizonsURL,
			Kind:        KindAPI,
			Subsystem:   "catalog/jpl",
			Description: "JPL Horizons API (small-body name resolution)",
			ApproxSize:  100_000,
			Enabled:     true,
			Timeout:     2 * time.Minute,
		},
		JPLHorizonsSPK: {
			ID:           JPLHorizonsSPK,
			URL:          horizonsURL,
			Kind:         KindAPI,
			Subsystem:    "ephemeris/jpl",
			Description:  "JPL Horizons API (small-body SPK kernel generation)",
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
			URL:             "https://raw.githubusercontent.com/mattiaverga/OpenNGC/36cb178a0f69dba8bfc03a99c10512831edf1c6b/database_files/",
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
			ID:        VIIRSAnnual,
			URL:       "https://www2.lightpollutionmap.info/data/v2/",
			Kind:      KindFile,
			Subsystem: "atlas",
			Description: "NASA VIIRS annual nighttime-lights composites (Black Marble VNP46A4/VJ146A4), " +
				"mirrored unauthenticated by lightpollutionmap.info, one raw GeoTIFF zip per year. " +
				"Source data CC0; credit \"Jurij Stare, www.lightpollutionmap.info\" and " +
				"\"NASA's Black Marble nighttime lights product\".",
			ApproxSize:      SizeVaries, // ~700 MB (2012) to ~1 GB (2025)
			Enabled:         true,
			DownloadTimeout: 60 * time.Minute,
			Mutable:         true, // past years are occasionally reprocessed in place
			Downloadable:    true,
		},
		PassbandBundle: {
			ID:              PassbandBundle,
			URL:             "https://github.com/TuSKan/astrogo/releases/download/passbands-v1/",
			Kind:            KindFile,
			Subsystem:       "skybrightness/dataset/passband",
			Description:     "Versioned, checksummed passband response-curve bundle (Johnson-Cousins, Sloan, Gaia, CIE, SQM)",
			ApproxSize:      2 << 20,
			Enabled:         true,
			DownloadTimeout: 2 * time.Minute,
			Mutable:         false, // pinned by semver plus manifest checksums
			Downloadable:    true,
			Files:           []string{"passbands-v1.tar.gz"},
		},
		GaiaStarMap: {
			ID:        GaiaStarMap,
			URL:       "https://github.com/TuSKan/astrogo/releases/download/starmap-v1/",
			Kind:      KindFile,
			Subsystem: "skybrightness/dataset/starlight",
			Description: "Integrated starlight aggregated from Gaia DR3 onto HEALPix order 8, " +
				"in Johnson V; one asset per magnitude cut",
			ApproxSize:      8 << 20,
			Enabled:         true,
			DownloadTimeout: 5 * time.Minute,
			Mutable:         false, // the cut and the order are in the filename
			Downloadable:    true,
			Files: []string{
				"starmap-o8-V-g6.txt.gz",
				"starmap-o8-V-all.txt.gz",
			},
		},

		CALSPEC: {
			ID:        CALSPEC,
			URL:       "https://archive.stsci.edu/hlsps/reference-atlases/cdbs/current_calspec/",
			Kind:      KindFile,
			Subsystem: "skybrightness/dataset/solar",
			Description: "STScI CALSPEC absolute flux standards; sun_reference_stis_002.fits is " +
				"the solar spectrum from 100 nm to 2.7 um (Colina, Bohlin & Castelli 1996)",
			ApproxSize:      3 << 20,
			Enabled:         true,
			DownloadTimeout: 5 * time.Minute,
			Mutable:         false, // versioned in the filename
			Downloadable:    true,
			Files:           []string{"sun_reference_stis_002.fits"},
		},
		WorldAtlas: {
			ID:        WorldAtlas,
			URL:       "https://datapub.gfz.de/download/10.5880.GFZ.1.4.2016.001/",
			Kind:      KindFile,
			Subsystem: "atlas",
			Description: "Falchi et al. 2016 World Atlas 2015 of artificial night sky brightness " +
				"(GFZ Data Services, DOI 10.5880/GFZ.1.4.2016.001). LICENSE: CC BY-NC 4.0, " +
				"non-commercial; attribute Falchi et al. (2016), Sci. Adv. 2, e1600377.",
			ApproxSize:      684_266_450, // World_Atlas_2015.zip, measured Content-Length
			Enabled:         true,
			DownloadTimeout: 60 * time.Minute,
			Mutable:         false, // DOI-versioned, frozen since 2019-11-18
			Downloadable:    true,
			Files:           []string{"World_Atlas_2015.zip"},
		},
		CopernicusEODATA: {
			ID:        CopernicusEODATA,
			URL:       copernicusEODATAURL,
			Kind:      KindFile,
			Subsystem: "atmosphere/dataset/cams",
			Description: "Copernicus Data Space Ecosystem EODATA object store " +
				"(https://eodata.dataspace.copernicus.eu) — a multi-product bucket (Sentinel, CLMS, " +
				"CAMS, ...) keyed by product prefix. astrogo reads the CAMS/GLOBAL/... prefix for " +
				"CAMS global analysis NetCDF-4 files. Requires Copernicus S3 credentials via the " +
				"AWS SDK default chain, and a blank import of remote/s3.",
			ApproxSize:      SizeVaries, // 1.3 MB (lnsp) to ~180 MB (a 137-level aerosol tracer)
			Enabled:         true,
			DownloadTimeout: 30 * time.Minute,
			Mutable:         true, // CAMS publishes new analysis cycles ~4x/day and reprocesses
			Downloadable:    true,
		},
	}
}
