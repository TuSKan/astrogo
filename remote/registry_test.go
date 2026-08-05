package remote

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestURLDefaults(t *testing.T) {
	t.Cleanup(Reset)

	u, err := URL(SIMBAD)
	if err != nil {
		t.Fatalf("URL(SIMBAD): %v", err)
	}

	if !strings.Contains(u, "simbad") {
		t.Errorf("unexpected SIMBAD URL: %s", u)
	}
}

func TestURLUnknownEndpoint(t *testing.T) {
	t.Cleanup(Reset)

	if _, err := URL("no.such.endpoint"); !errors.Is(err, ErrUnknownEndpoint) {
		t.Errorf("expected ErrUnknownEndpoint, got %v", err)
	}
}

func TestDisableEnable(t *testing.T) {
	t.Cleanup(Reset)

	Disable(SIMBAD)

	if _, err := URL(SIMBAD); !errors.Is(err, ErrEndpointDisabled) {
		t.Errorf("expected ErrEndpointDisabled, got %v", err)
	}

	// Other endpoints unaffected.
	if _, err := URL(GaiaTAP); err != nil {
		t.Errorf("GaiaTAP should remain enabled: %v", err)
	}

	Enable(SIMBAD)

	if _, err := URL(SIMBAD); err != nil {
		t.Errorf("re-enabled SIMBAD should resolve: %v", err)
	}
}

func TestSetOffline(t *testing.T) {
	t.Cleanup(Reset)

	SetOffline(true)

	for _, ep := range Endpoints() {
		if _, err := URL(ep.ID); !errors.Is(err, ErrOffline) {
			t.Errorf("endpoint %s: expected ErrOffline, got %v", ep.ID, err)
		}
	}

	if !Offline() {
		t.Error("Offline() should report true")
	}
}

func TestSetURLOverride(t *testing.T) {
	t.Cleanup(Reset)

	if err := SetURL(SIMBAD, "http://mirror.example/tap"); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	u, err := URL(SIMBAD)
	if err != nil {
		t.Fatalf("URL after override: %v", err)
	}

	if u != "http://mirror.example/tap" {
		t.Errorf("override not applied, got %s", u)
	}

	if err := SetURL("no.such.endpoint", "http://x"); !errors.Is(err, ErrUnknownEndpoint) {
		t.Errorf("expected ErrUnknownEndpoint, got %v", err)
	}
}

func TestReset(t *testing.T) {
	Disable(SIMBAD)
	SetOffline(true)
	EnableDownloads(NAIFSPK, 1)
	Reset()

	if Offline() {
		t.Error("Reset should clear offline mode")
	}

	if _, err := URL(SIMBAD); err != nil {
		t.Errorf("Reset should re-enable SIMBAD: %v", err)
	}

	if ok, _ := DownloadsEnabled(NAIFSPK); ok {
		t.Error("Reset should revoke download consent")
	}
}

func TestDownloadConsentDefaultDeny(t *testing.T) {
	t.Cleanup(Reset)

	err := CheckDownload(NAIFSPK, "de442.bsp", 115<<20)
	if !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("expected ErrDownloadDenied by default, got %v", err)
	}

	// The error must be actionable: name the file, a size, and the enable call.
	msg := err.Error()
	for _, want := range []string{"de442.bsp", "MB", "EnableDownloads", "NAIFSPK"} {
		if !strings.Contains(msg, want) {
			t.Errorf("denial message missing %q: %s", want, msg)
		}
	}
}

func TestDownloadConsentEnableAndLimit(t *testing.T) {
	t.Cleanup(Reset)

	EnableDownloads(NAIFSPK, 50<<20)

	if err := CheckDownload(NAIFSPK, "de440s.bsp", 32<<20); err != nil {
		t.Errorf("32MB under a 50MB limit should pass: %v", err)
	}

	err := CheckDownload(NAIFSPK, "de442.bsp", 115<<20)
	if !errors.Is(err, ErrDownloadDenied) {
		t.Errorf("115MB over a 50MB limit should be denied, got %v", err)
	}

	// Unlimited.
	EnableDownloads(NAIFSPK, 0)

	if err := CheckDownload(NAIFSPK, "de441_part-1.bsp", 3<<30); err != nil {
		t.Errorf("unlimited consent should pass any size: %v", err)
	}

	// Unknown size passes an enabled endpoint (re-checked with the exact
	// Content-Length once headers arrive).
	EnableDownloads(NAIFSPK, 50<<20)

	if err := CheckDownload(NAIFSPK, "unknown.bsp", -1); err != nil {
		t.Errorf("unknown size should defer to the Content-Length check: %v", err)
	}

	DisableDownloads(NAIFSPK)

	if err := CheckDownload(NAIFSPK, "de440s.bsp", 1); !errors.Is(err, ErrDownloadDenied) {
		t.Errorf("DisableDownloads should restore denial, got %v", err)
	}
}

func TestEnableAllDownloadsAndDisableAllDownloads(t *testing.T) {
	t.Cleanup(Reset)

	EnableAllDownloads(50 << 20)

	for _, id := range []EndpointID{IERSFinals2000A, NAIFSPK, NAIFLSK, OpenNGC, JPLHorizons} {
		ok, maxSize := DownloadsEnabled(id)
		if !ok {
			t.Errorf("EnableAllDownloads: %s: expected DownloadsOK=true", id)
		}

		if maxSize != 50<<20 {
			t.Errorf("EnableAllDownloads: %s: MaxDownloadSize = %d, want %d", id, maxSize, 50<<20)
		}
	}

	// A non-Downloadable KindAPI endpoint has no download-consent gate —
	// EnableAllDownloads must not touch it either way.
	if ok, _ := DownloadsEnabled(SIMBAD); ok {
		t.Error("EnableAllDownloads must not grant consent to a non-Downloadable endpoint")
	}

	DisableAllDownloads()

	for _, id := range []EndpointID{IERSFinals2000A, NAIFSPK, NAIFLSK, OpenNGC, JPLHorizons} {
		if ok, maxSize := DownloadsEnabled(id); ok || maxSize != 0 {
			t.Errorf("DisableAllDownloads: %s: expected DownloadsOK=false, MaxDownloadSize=0, got ok=%v maxSize=%d", id, ok, maxSize)
		}
	}
}

// TestEnableAllDownloadsCoversHorizons is a regression test for a real
// consent-scope bug: JPLHorizons is a KindAPI endpoint whose small-body
// SPK generation is nonetheless a genuine file download
// (ephemeris/jpl/spk.CacheAPI), gated the same as any KindFile endpoint
// — but EnableAllDownloads used to only ever grant KindFile endpoints,
// so a caller who called EnableAllDownloads reasonably believed every
// download was unblocked while every asteroid/comet ephemeris fetch
// still silently failed with ErrDownloadDenied.
func TestEnableAllDownloadsCoversHorizons(t *testing.T) {
	t.Cleanup(Reset)

	EnableAllDownloads(0)

	if ok, _ := DownloadsEnabled(JPLHorizons); !ok {
		t.Fatal("EnableAllDownloads(0) must grant consent to JPLHorizons — its small-body SPK generation is a real file download")
	}
}

// TestDownloadableEndpointsAreExactlyTheExpectedSet is a golden-list
// assertion: a future endpoint that can genuinely perform a download
// must have Downloadable set explicitly, or EnableAllDownloads silently
// leaves it ungranted — the same failure mode this file's other tests
// exist to catch for JPLHorizons.
func TestDownloadableEndpointsAreExactlyTheExpectedSet(t *testing.T) {
	want := map[EndpointID]bool{
		IERSFinals2000A: true,
		NAIFSPK:         true,
		NAIFLSK:         true,
		OpenNGC:         true,
		JPLHorizons:     true,
		WorldAtlas:      true,
		VIIRSAnnual:     true,
	}

	for _, ep := range Endpoints() {
		if ep.Downloadable != want[ep.ID] {
			t.Errorf("Endpoint %s: Downloadable = %v, want %v", ep.ID, ep.Downloadable, want[ep.ID])
		}
	}
}

// TestConstName is a golden-list check: every registered EndpointID must
// map back to its own exported Go constant name, so a download-denied
// error message (the only caller of constName) always shows copy-pasteable
// code rather than the raw registry key. An id with no case falls through
// to the raw string — exercised via a made-up id that isn't registered.
func TestConstName(t *testing.T) {
	want := map[EndpointID]string{
		IERSFinals2000A: "IERSFinals2000A",
		NAIFSPK:         "NAIFSPK",
		NAIFLSK:         "NAIFLSK",
		JPLHorizons:     "JPLHorizons",
		JPLSBDB:         "JPLSBDB",
		JPLSBDBQuery:    "JPLSBDBQuery",
		SIMBAD:          "SIMBAD",
		VizieR:          "VizieR",
		GaiaTAP:         "GaiaTAP",
		MAST:            "MAST",
		CelesTrak:       "CelesTrak",
		FINK:            "FINK",
		LightPollution:  "LightPollution",
		OpenNGC:         "OpenNGC",
		Nominatim:       "Nominatim",
		OpenElevation:   "OpenElevation",
		WorldAtlas:      "WorldAtlas",
		VIIRSAnnual:     "VIIRSAnnual",
	}

	for id, name := range want {
		if got := constName(id); got != name {
			t.Errorf("constName(%s) = %q, want %q", id, got, name)
		}
	}

	if got := constName("no.such.endpoint"); got != "no.such.endpoint" {
		t.Errorf("constName(unregistered) = %q, want the raw id back", got)
	}
}

var errKernelsForbidden = errors.New("kernels forbidden here")

func TestCustomPolicy(t *testing.T) {
	t.Cleanup(Reset)

	SetPolicy(func(ep Endpoint, _ int64) error {
		if ep.ID == NAIFSPK {
			return errKernelsForbidden
		}

		return nil
	})

	err := CheckDownload(NAIFSPK, "de442.bsp", 115<<20)
	if !errors.Is(err, ErrDownloadDenied) || !strings.Contains(err.Error(), "kernels forbidden here") {
		t.Errorf("custom policy denial not surfaced: %v", err)
	}

	// Policy replaces per-endpoint consent: LSK passes without EnableDownloads.
	if err := CheckDownload(NAIFLSK, "naif0012.tls", 6000); err != nil {
		t.Errorf("policy-allowed download should pass: %v", err)
	}

	SetPolicy(nil)

	if err := CheckDownload(NAIFLSK, "naif0012.tls", 6000); !errors.Is(err, ErrDownloadDenied) {
		t.Errorf("nil policy should restore per-endpoint consent, got %v", err)
	}
}

func TestEndpointsSnapshot(t *testing.T) {
	t.Cleanup(Reset)

	eps := Endpoints()
	if len(eps) < 12 {
		t.Fatalf("expected the full endpoint table, got %d entries", len(eps))
	}

	// Sorted by ID and mutation of the snapshot must not affect the registry.
	for i := 1; i < len(eps); i++ {
		if eps[i-1].ID >= eps[i].ID {
			t.Errorf("Endpoints not sorted: %s >= %s", eps[i-1].ID, eps[i].ID)
		}
	}

	eps[0].URL = "http://mutated.example"

	if fresh, _ := Lookup(eps[0].ID); fresh.URL == "http://mutated.example" {
		t.Error("mutating the snapshot leaked into the registry")
	}
}

func TestLookupFilesSliceIsNotShared(t *testing.T) {
	t.Cleanup(Reset)

	ep, ok := Lookup(OpenNGC)
	if !ok || len(ep.Files) == 0 {
		t.Fatalf("Lookup(OpenNGC) = %+v, %v, want a populated Files manifest", ep, ok)
	}

	original := ep.Files[0]
	ep.Files[0] = "mutated.csv"

	if fresh, _ := Lookup(OpenNGC); fresh.Files[0] != original {
		t.Error("mutating a Lookup result's Files slice leaked into the registry")
	}
}

func TestRegistryConcurrency(t *testing.T) {
	t.Cleanup(Reset)

	var wg sync.WaitGroup

	for range 8 {
		wg.Go(func() {
			for range 100 {
				_ = SetURL(SIMBAD, "http://a.example")
				_, _ = URL(SIMBAD)
				_ = Endpoints()

				Disable(GaiaTAP)
				Enable(GaiaTAP)
				EnableDownloads(NAIFSPK, 1<<20)

				_, _ = DownloadsEnabled(NAIFSPK)

				SetOffline(false)
			}
		})
	}

	wg.Wait()
}
