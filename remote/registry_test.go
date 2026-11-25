package remote

import (
	"errors"
	"regexp"
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
	SetPolicy(func(Endpoint, int64) error { return errKernelsForbidden })
	Reset()

	if Offline() {
		t.Error("Reset should clear offline mode")
	}

	if _, err := URL(SIMBAD); err != nil {
		t.Errorf("Reset should re-enable SIMBAD: %v", err)
	}

	if err := CheckDownload(NAIFSPK, "de440s.bsp", 32<<20); err != nil {
		t.Errorf("Reset should clear the download policy: %v", err)
	}
}

// TestDownloadsAreAllowedByDefault: with no policy, every download is
// allowed, of any size, on the default client and on a new one.
func TestDownloadsAreAllowedByDefault(t *testing.T) {
	t.Cleanup(Reset)

	if err := CheckDownload(NAIFSPK, "de441_part-1.bsp", 3<<30); err != nil {
		t.Errorf("a 3 GB kernel with no policy set: %v, want nil", err)
	}

	if err := NewClient().CheckDownload(NAIFSPK, "de441_part-1.bsp", 3<<30); err != nil {
		t.Errorf("a new client: %v, want nil", err)
	}
}

// TestPolicyCapsTheSize is the size cap a policy expresses: under it passes,
// over it is refused with a message naming the file, its size and SetPolicy,
// and an unknown size passes, to be checked again against the size the source
// reports. SetPolicy(nil) allows everything again.
func TestPolicyCapsTheSize(t *testing.T) {
	t.Cleanup(Reset)

	SetPolicy(func(_ Endpoint, size int64) error {
		if size > 50<<20 {
			return errKernelsForbidden
		}

		return nil
	})

	if err := CheckDownload(NAIFSPK, "de440s.bsp", 32<<20); err != nil {
		t.Errorf("32 MB under a 50 MB cap should pass: %v", err)
	}

	err := CheckDownload(NAIFSPK, "de442.bsp", 115<<20)
	if !errors.Is(err, ErrDownloadDenied) || !errors.Is(err, errKernelsForbidden) {
		t.Fatalf("115 MB over a 50 MB cap: %v, want ErrDownloadDenied wrapping the policy's reason", err)
	}

	for _, want := range []string{"de442.bsp", "MB", "SetPolicy"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal message missing %q: %s", want, err)
		}
	}

	if err := CheckDownload(NAIFSPK, "unknown.bsp", -1); err != nil {
		t.Errorf("an unknown size should defer to the size the source reports: %v", err)
	}

	SetPolicy(nil)

	if err := CheckDownload(NAIFSPK, "de442.bsp", 115<<20); err != nil {
		t.Errorf("SetPolicy(nil) should allow every download again: %v", err)
	}
}

// A golden list: an endpoint that can genuinely download must set
// Downloadable explicitly, which is what says the download policy is asked
// about it and the log reports it.
func TestDownloadableEndpointsAreExactlyTheExpectedSet(t *testing.T) {
	want := map[EndpointID]bool{
		IERSFinals2000A:       true,
		NAIFSPK:               true,
		NAIFLSK:               true,
		NAIFPCK:               true,
		OpenNGC:               true,
		MPCObsCodes:           true,
		MPCORB:                true,
		JPLHorizonsSPK:        true,
		VIIRSAnnual:           true,
		CopernicusEODATA:      true,
		CALSPEC:               true,
		GaiaStarMap:           true,
		MPIMainzCrossSections: true,
		SFDDustMap:            true,
		CIELuminosity:         true,
	}

	for _, ep := range Endpoints() {
		if ep.Downloadable != want[ep.ID] {
			t.Errorf("Endpoint %s: Downloadable = %v, want %v", ep.ID, ep.Downloadable, want[ep.ID])
		}
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

	// The policy decides per endpoint: LSK passes.
	if err := CheckDownload(NAIFLSK, "naif0012.tls", 6000); err != nil {
		t.Errorf("policy-allowed download should pass: %v", err)
	}

	SetPolicy(nil)

	if err := CheckDownload(NAIFSPK, "de442.bsp", 115<<20); err != nil {
		t.Errorf("a nil policy should allow every download, got %v", err)
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
				SetPolicy(nil)

				_ = CheckDownload(NAIFSPK, "de440s.bsp", 1)

				SetOffline(false)
			}
		})
	}

	wg.Wait()
}

// A key must never arrive pre-encoded.
//
// The MPI-Mainz ozone file has a space in its name. Writing it as %20 in the
// registry looked like the careful thing to do and was the opposite: the blob
// layer percent-encodes what it is given, so the percent sign itself became
// %25 and the request went out for %2520. The server answered 404, which reads
// as a missing file rather than as a double-encoded one, and the cause took
// longer to find than it should have.
//
// Encoding belongs to the layer that builds the URL. A key holds the literal
// name, spaces and all.
func TestEndpointFilesAreNotPreEncoded(t *testing.T) {
	t.Parallel()

	// %XX where XX is hex. Anything matching is a key that has already been
	// through an encoder once.
	encoded := regexp.MustCompile(`%[0-9A-Fa-f]{2}`)

	for _, ep := range Endpoints() {
		for _, name := range ep.Files {
			if encoded.MatchString(name) {
				t.Errorf("endpoint %s file %q is percent-encoded; keys hold literal names "+
					"and the blob layer encodes them", ep.ID, name)
			}
		}
	}
}

// The same rule for the endpoint URLs' path segments, which are joined to a
// key and handed to file.OpenFS.
func TestEndpointURLsHaveNoDoubleEncoding(t *testing.T) {
	t.Parallel()

	for _, ep := range Endpoints() {
		if strings.Contains(ep.URL, "%25") {
			t.Errorf("endpoint %s URL contains %%25, which is a percent sign that has "+
				"already been encoded once: %s", ep.ID, ep.URL)
		}
	}
}
