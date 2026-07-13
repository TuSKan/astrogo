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
