package remote

import (
	"fmt"
	"slices"
	"sort"
	"sync"
)

// registry holds the process-wide endpoint configuration. It is statically
// initialized (no init() computation) and guarded by a single RWMutex; every
// exported accessor below is safe for concurrent use.
//
//nolint:gochecknoglobals // process-wide endpoint configuration is this package's purpose
var (
	regMu     sync.RWMutex
	endpoints = defaultEndpoints()
	offline   bool
	policy    Policy
)

// Endpoints returns a snapshot of every registered endpoint, sorted by ID.
func Endpoints() []Endpoint {
	regMu.RLock()
	defer regMu.RUnlock()

	out := make([]Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		out = append(out, cloneEndpoint(ep))
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })

	return out
}

// cloneEndpoint copies ep's Files slice so a caller mutating a returned
// Endpoint (or its Files) can never corrupt the registry's own copy —
// every other Endpoint field is a value type and already snapshot-safe
// via Go's plain struct copy.
func cloneEndpoint(ep Endpoint) Endpoint {
	ep.Files = slices.Clone(ep.Files)

	return ep
}

// Lookup returns the endpoint registered under id.
func Lookup(id EndpointID) (Endpoint, bool) {
	regMu.RLock()
	defer regMu.RUnlock()

	ep, ok := endpoints[id]

	return cloneEndpoint(ep), ok
}

// Enable re-enables access to the given endpoints (the default state).
func Enable(ids ...EndpointID) {
	setEnabled(true, ids)
}

// Disable blocks all access to the given endpoints: any request against
// them fails with ErrEndpointDisabled until re-enabled.
func Disable(ids ...EndpointID) {
	setEnabled(false, ids)
}

func setEnabled(enabled bool, ids []EndpointID) {
	regMu.Lock()
	defer regMu.Unlock()

	for _, id := range ids {
		if ep, ok := endpoints[id]; ok {
			ep.Enabled = enabled
			endpoints[id] = ep
		}
	}
}

// SetURL overrides an endpoint's base URL, e.g. to point at a mirror or an
// internal proxy. Returns ErrUnknownEndpoint for an unregistered id.
func SetURL(id EndpointID, url string) error {
	regMu.Lock()
	defer regMu.Unlock()

	ep, ok := endpoints[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	ep.URL = url
	endpoints[id] = ep

	return nil
}

// SetOffline toggles global offline mode. While offline, every endpoint
// access — API call or download — fails with ErrOffline.
func SetOffline(off bool) {
	regMu.Lock()
	defer regMu.Unlock()

	offline = off
}

// Offline reports whether global offline mode is active.
func Offline() bool {
	regMu.RLock()
	defer regMu.RUnlock()

	return offline
}

// Reset restores the default registry state: every endpoint at its
// built-in URL, downloads disabled, online, no custom policy. It does not
// touch the data directory (see DataDir).
//
// Reset discards consent a broader scope — a package TestMain — granted
// for the whole binary, so a test using it can break tests that run
// after. Prefer Capture/Restore or WithScope for scoped setup.
func Reset() {
	regMu.Lock()
	defer regMu.Unlock()

	endpoints = defaultEndpoints()
	offline = false
	policy = nil
}

// URL is the single gate every network call site goes through: it returns
// the endpoint's (possibly overridden) base URL, or an error explaining why
// the endpoint may not be contacted — ErrOffline, ErrEndpointDisabled, or
// ErrUnknownEndpoint.
func URL(id EndpointID) (string, error) {
	regMu.RLock()
	defer regMu.RUnlock()

	if offline {
		return "", fmt.Errorf("%w (endpoint %q)", ErrOffline, id)
	}

	ep, ok := endpoints[id]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	if !ep.Enabled {
		return "", fmt.Errorf("%w: %q", ErrEndpointDisabled, id)
	}

	return ep.URL, nil
}

// EnableDownloads grants file-download consent, capping a single download
// at maxSize bytes (0 means unlimited). With no ids it applies to every
// Downloadable endpoint; otherwise only to the ones named. Downloads are
// always denied until this is called — astrogo never auto-downloads.
//
//	remote.EnableDownloads(200<<20, remote.NAIFSPK, remote.NAIFLSK)
//	remote.EnableDownloads(0) // everything, no size cap
//
// Naming an endpoint that cannot download (SIMBAD, VizieR, ...) does
// nothing: it has no consent gate to open.
func EnableDownloads(maxSize int64, ids ...EndpointID) {
	setConsent(true, maxSize, ids)
}

// DisableDownloads revokes file-download consent — for every Downloadable
// endpoint with no ids, or only the ones named. This is the default state.
func DisableDownloads(ids ...EndpointID) {
	setConsent(false, 0, ids)
}

// setConsent applies a consent decision. An empty ids means every
// Downloadable endpoint; naming ids explicitly still skips those with no
// download path at all, so consent is never recorded against an endpoint
// that would never consult it.
func setConsent(ok bool, maxSize int64, ids []EndpointID) {
	regMu.Lock()
	defer regMu.Unlock()

	if len(ids) == 0 {
		ids = make([]EndpointID, 0, len(endpoints))
		for id := range endpoints {
			ids = append(ids, id)
		}
	}

	for _, id := range ids {
		ep, found := endpoints[id]
		if !found || !ep.Downloadable {
			continue
		}

		ep.DownloadsOK = ok
		ep.MaxDownloadSize = maxSize
		endpoints[id] = ep
	}
}

// DownloadsEnabled reports whether downloads are enabled for id and the
// configured per-download size cap (0 = unlimited).
func DownloadsEnabled(id EndpointID) (ok bool, maxSize int64) {
	regMu.RLock()
	defer regMu.RUnlock()

	ep, found := endpoints[id]
	if !found {
		return false, 0
	}

	return ep.DownloadsOK, ep.MaxDownloadSize
}

// Policy decides whether a file download may proceed. size is the exact
// Content-Length when known, the endpoint's ApproxSize otherwise, or -1
// when neither is available. Returning a non-nil error aborts the download,
// wrapped in ErrDownloadDenied.
type Policy func(ep Endpoint, size int64) error

// SetPolicy installs a custom download-consent policy that replaces the
// per-endpoint EnableDownloads checks entirely. Pass nil to restore the
// default per-endpoint consent behavior.
func SetPolicy(p Policy) {
	regMu.Lock()
	defer regMu.Unlock()

	policy = p
}

// CheckDownload applies the consent configuration to a prospective
// download of the given size (semantics as in Policy), returning nil when
// it may proceed. GetFile calls this itself; it is exported for
// file-producing paths that bypass GetFile — JPLHorizonsSPK generation,
// whose kernel arrives base64-encoded inside a JSON response.
func CheckDownload(id EndpointID, name string, size int64) error {
	regMu.RLock()

	ep, ok := endpoints[id]
	pol := policy

	regMu.RUnlock()

	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownEndpoint, id)
	}

	if pol != nil {
		if err := pol(ep, size); err != nil {
			return fmt.Errorf("%w: %s from %s: %w", ErrDownloadDenied, name, ep.URL, err)
		}

		return nil
	}

	if !ep.DownloadsOK {
		return fmt.Errorf(
			"%w: %s (%s from %s); astrogo never downloads without consent — call remote.EnableDownloads(maxSize, remote.%s) or pre-seed the file (see README \"Data downloads & offline usage\")",
			ErrDownloadDenied, name, sizeLabel(size), ep.URL, constName(id),
		)
	}

	if ep.MaxDownloadSize > 0 && size > ep.MaxDownloadSize {
		return fmt.Errorf(
			"%w: %s is %s, above the %s limit set by remote.EnableDownloads(..., remote.%s)",
			ErrDownloadDenied, name, sizeLabel(size), sizeLabel(ep.MaxDownloadSize), constName(id),
		)
	}

	return nil
}

// sizeLabel formats a byte count for error messages; negative/unknown sizes
// read as "size unknown".
func sizeLabel(size int64) string {
	switch {
	case size < 0:
		return "size unknown"
	case size >= 1<<30:
		return fmt.Sprintf("~%.1f GB", float64(size)/(1<<30))
	case size >= 1<<20:
		return fmt.Sprintf("~%.0f MB", float64(size)/(1<<20))
	case size >= 1<<10:
		return fmt.Sprintf("~%.0f KB", float64(size)/(1<<10))
	default:
		return fmt.Sprintf("%d bytes", size)
	}
}

// constName maps an EndpointID to its exported Go constant name so error
// messages show copy-pasteable code.
func constName(id EndpointID) string {
	switch id {
	case IERSFinals2000A:
		return "IERSFinals2000A"
	case NAIFSPK:
		return "NAIFSPK"
	case NAIFLSK:
		return "NAIFLSK"
	case JPLHorizons:
		return "JPLHorizons"
	case JPLHorizonsSPK:
		return "JPLHorizonsSPK"
	case JPLSBDB:
		return "JPLSBDB"
	case JPLSBDBQuery:
		return "JPLSBDBQuery"
	case SIMBAD:
		return "SIMBAD"
	case VizieR:
		return "VizieR"
	case GaiaTAP:
		return "GaiaTAP"
	case ESOSkyCalc:
		return "ESOSkyCalc"
	case GaiaAIP:
		return "GaiaAIP"
	case GaiaAIPAsync:
		return "GaiaAIPAsync"
	case IRSADust:
		return "IRSADust"
	case SVOFilterProfile:
		return "SVOFilterProfile"
	case GaiaStarMap:
		return "GaiaStarMap"
	case MPIMainzCrossSections:
		return "MPIMainzCrossSections"
	case MAST:
		return "MAST"
	case CelesTrak:
		return "CelesTrak"
	case FINK:
		return "FINK"
	case LightPollution:
		return "LightPollution"
	case OpenNGC:
		return "OpenNGC"
	case Nominatim:
		return "Nominatim"
	case OpenElevation:
		return "OpenElevation"
	case CALSPEC:
		return "CALSPEC"
	case WorldAtlas:
		return "WorldAtlas"
	case VIIRSAnnual:
		return "VIIRSAnnual"
	case PassbandBundle:
		return "PassbandBundle"
	case CopernicusEODATA:
		return "CopernicusEODATA"
	default:
		return string(id)
	}
}
