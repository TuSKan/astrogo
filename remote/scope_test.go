package remote

import (
	"errors"
	"testing"
)

var errCustomPolicyForTest = errors.New("custom policy")

// TestCaptureRestoreFullSnapshot verifies Capture()/Restore() round-trips
// every field the doc comment promises: per-endpoint URL/Enabled/
// DownloadsOK/MaxDownloadSize, offline mode, the custom Policy, and the
// data directory.
func TestCaptureRestoreFullSnapshot(t *testing.T) {
	t.Cleanup(Reset)
	t.Cleanup(func() { SetDataDir("") })

	// SetDataDir (not SetDataDirPath) stores the given string verbatim, no
	// path resolution — the exact round trip this test wants to pin,
	// distinct from TestDataDirEnvOverride's real path-resolution check
	// below.
	SetDataDir("before")
	EnableDownloads(10<<20, NAIFSPK)

	snap := Capture()

	// Mutate everything the snapshot should restore.
	SetDataDir("after")
	DisableDownloads(NAIFSPK)

	if err := SetURL(NAIFSPK, "https://example.invalid/mutated"); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	Disable(NAIFSPK)
	SetOffline(true)

	SetPolicy(func(Endpoint, int64) error { return errCustomPolicyForTest })

	snap.Restore()

	if got := DataDirURL(); got != "before" {
		t.Errorf("DataDirURL after Restore = %q, want %q", got, "before")
	}

	if ok, maxSize := DownloadsEnabled(NAIFSPK); !ok || maxSize != 10<<20 {
		t.Errorf("DownloadsEnabled(NAIFSPK) after Restore = (%v, %d), want (true, %d)", ok, maxSize, 10<<20)
	}

	ep, ok := Lookup(NAIFSPK)
	if !ok {
		t.Fatal("Lookup(NAIFSPK) after Restore: not found")
	}

	if !ep.Enabled {
		t.Error("NAIFSPK.Enabled after Restore = false, want true (restored)")
	}

	if got, err := URL(NAIFSPK); err != nil || got == "https://example.invalid/mutated" {
		t.Errorf("URL(NAIFSPK) after Restore = (%q, %v), want the original built-in URL", got, err)
	}

	if Offline() {
		t.Error("Offline() after Restore = true, want false (restored)")
	}

	if err := CheckDownload(NAIFSPK, "x", 1); err != nil {
		t.Errorf("CheckDownload after Restore should use the restored (nil) policy, not the mutated one: %v", err)
	}
}

// TestCaptureSubsetLeavesOtherEndpointsAlone is the scenario that actually
// motivated this primitive: a scope captured for one endpoint must not
// clobber consent a broader scope (e.g. a package TestMain) granted for a
// different endpoint.
func TestCaptureSubsetLeavesOtherEndpointsAlone(t *testing.T) {
	t.Cleanup(Reset)

	EnableDownloads(0, NAIFLSK) // stands in for a TestMain-wide grant

	snap := Capture(NAIFSPK)

	DisableDownloads(NAIFSPK)
	// A test mistakenly (or legitimately, mid-test) disabling NAIFLSK too —
	// Restore() below must NOT touch it, since it wasn't captured.
	DisableDownloads(NAIFLSK)

	snap.Restore()

	if ok, _ := DownloadsEnabled(NAIFSPK); ok {
		t.Error("Restore() re-enabled NAIFSPK, but it was never disabled before Capture — DisableDownloads should have stuck")
	}

	if ok, _ := DownloadsEnabled(NAIFLSK); ok {
		t.Error("Restore(subset) touched NAIFLSK, which was outside the captured subset — it should have stayed as this test left it")
	}
}

// TestWithScopeRestoresOnPanic verifies WithScope restores its captured
// configuration even when fn panics, via defer.
func TestWithScopeRestoresOnPanic(t *testing.T) {
	t.Cleanup(Reset)

	EnableDownloads(5<<20, NAIFSPK)

	func() {
		defer func() { _ = recover() }()

		WithScope(func() {
			DisableDownloads(NAIFSPK)

			panic("boom")
		})
	}()

	if ok, maxSize := DownloadsEnabled(NAIFSPK); !ok || maxSize != 5<<20 {
		t.Errorf("DownloadsEnabled(NAIFSPK) after a panicking WithScope = (%v, %d), want (true, %d) — Restore must run via defer", ok, maxSize, 5<<20)
	}
}

// DataDir's precedence chain: an explicit SetDataDir wins over
// DataDirEnv, which wins over the OS default. Both are bucket URLs — the
// environment variable is taken verbatim, not converted from a path, so
// it can point at any backend a driver serves.
func TestDataDirEnvOverride(t *testing.T) {
	t.Cleanup(func() { SetDataDir("") })

	const envURL = "s3://cache-from-env"

	SetDataDir("")
	t.Setenv(DataDirEnv, envURL)

	if got := DataDirURL(); got != envURL {
		t.Errorf("DataDirURL() with only %s set = %q, want %q", DataDirEnv, got, envURL)
	}

	const explicitURL = "file:///explicit?create_dir=true"

	SetDataDir(explicitURL)

	if got := DataDirURL(); got != explicitURL {
		t.Errorf("DataDirURL() with SetDataDir set = %q, want %q (explicit must win over %s)", got, explicitURL, DataDirEnv)
	}
}
