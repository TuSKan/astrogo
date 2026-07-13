package remote

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

	// JPLSBDB is the JPL Small-Body Database query API (catalog/sbdb).
	JPLSBDB EndpointID = "jpl.sbdb"

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

	// OpenNGC is the OpenNGC catalog source on GitHub, contacted only at
	// development time by `go generate ./catalog/openngc/...` — never at
	// library runtime.
	OpenNGC EndpointID = "openngc.github"
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
)

// Endpoint describes one remote service: where it lives, what it is for,
// and how much data a request against it typically moves.
type Endpoint struct {
	// ID is the registry key.
	ID EndpointID
	// URL is the endpoint's base URL. Override with SetURL to point at a
	// mirror or proxy.
	URL string
	// Kind is KindAPI or KindFile.
	Kind Kind
	// Subsystem names the astrogo package family using this endpoint.
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
}

// SizeVaries marks an endpoint whose per-fetch size cannot be usefully
// approximated.
const SizeVaries int64 = -1

// defaultEndpoints is the built-in registry table. URLs here are the single
// source of truth for where astrogo connects — packages resolve them via
// URL(id) at request-build time.
func defaultEndpoints() map[EndpointID]Endpoint {
	return map[EndpointID]Endpoint{
		IERSFinals2000A: {
			ID:          IERSFinals2000A,
			URL:         "https://datacenter.iers.org/data/9/finals2000A.all",
			Kind:        KindFile,
			Subsystem:   "iers",
			Description: "IERS Earth-orientation parameters (finals2000A.all)",
			ApproxSize:  3_800_000,
			Enabled:     true,
		},
		NAIFSPK: {
			ID:          NAIFSPK,
			URL:         "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/spk/",
			Kind:        KindFile,
			Subsystem:   "ephemeris/jpl",
			Description: "NASA NAIF planetary ephemeris SPK kernels (de440s ~32 MB, de440/de442 ~115 MB, de441 multi-GB)",
			ApproxSize:  SizeVaries,
			Enabled:     true,
		},
		NAIFLSK: {
			ID:          NAIFLSK,
			URL:         "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/",
			Kind:        KindFile,
			Subsystem:   "ephemeris/jpl",
			Description: "NASA NAIF leap-second kernel (naif0012.tls)",
			ApproxSize:  6_000,
			Enabled:     true,
		},
		JPLHorizons: {
			ID:          JPLHorizons,
			URL:         "https://ssd.jpl.nasa.gov/api/horizons.api",
			Kind:        KindAPI,
			Subsystem:   "ephemeris/jpl, catalog/jpl",
			Description: "JPL Horizons API (name resolution and small-body SPK generation)",
			ApproxSize:  SizeVaries,
			Enabled:     true,
		},
		JPLSBDB: {
			ID:          JPLSBDB,
			URL:         "https://ssd-api.jpl.nasa.gov/sbdb.api",
			Kind:        KindAPI,
			Subsystem:   "catalog/sbdb",
			Description: "JPL Small-Body Database query API",
			ApproxSize:  100_000,
			Enabled:     true,
		},
		SIMBAD: {
			ID:          SIMBAD,
			URL:         "http://simbad.cds.unistra.fr/simbad/sim-tap/sync",
			Kind:        KindAPI,
			Subsystem:   "catalog/simbad",
			Description: "CDS SIMBAD TAP service",
			ApproxSize:  100_000,
			Enabled:     true,
		},
		VizieR: {
			ID:          VizieR,
			URL:         "http://tapvizier.u-strasbg.fr/TAPVizieR/tap/sync",
			Kind:        KindAPI,
			Subsystem:   "catalog/vizier",
			Description: "CDS VizieR TAP service",
			ApproxSize:  100_000,
			Enabled:     true,
		},
		GaiaTAP: {
			ID:          GaiaTAP,
			URL:         "https://gea.esac.esa.int/tap-server/tap/sync",
			Kind:        KindAPI,
			Subsystem:   "catalog/gaia",
			Description: "ESA Gaia archive TAP service",
			ApproxSize:  100_000,
			Enabled:     true,
		},
		MAST: {
			ID:          MAST,
			URL:         "https://mast.stsci.edu/api/v0/invoke",
			Kind:        KindAPI,
			Subsystem:   "catalog/mast",
			Description: "STScI MAST invoke API",
			ApproxSize:  100_000,
			Enabled:     true,
		},
		CelesTrak: {
			ID:          CelesTrak,
			URL:         "https://celestrak.org/NORAD/elements/gp.php",
			Kind:        KindAPI,
			Subsystem:   "catalog/norad",
			Description: "CelesTrak GP element sets (TLEs)",
			ApproxSize:  100_000,
			Enabled:     true,
		},
		FINK: {
			ID:          FINK,
			URL:         "https://api.ztf.fink-portal.org/api/v1/ssoft",
			Kind:        KindAPI,
			Subsystem:   "catalog/fink",
			Description: "FINK broker ZTF solar-system object feature table",
			ApproxSize:  SizeVaries,
			Enabled:     true,
		},
		LightPollution: {
			ID:          LightPollution,
			URL:         "https://www.lightpollutionmap.info/QueryRaster/",
			Kind:        KindAPI,
			Subsystem:   "lightpollution",
			Description: "lightpollutionmap.info raster query (World Atlas 2015)",
			ApproxSize:  1_000,
			Enabled:     true,
		},
		OpenNGC: {
			ID:          OpenNGC,
			URL:         "https://raw.githubusercontent.com/mattiaverga/OpenNGC",
			Kind:        KindFile,
			Subsystem:   "catalog/openngc (go generate only)",
			Description: "OpenNGC catalog source CSVs (development-time regeneration)",
			ApproxSize:  2_000_000,
			Enabled:     true,
		},
	}
}
