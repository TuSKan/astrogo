package remote

import (
	"errors"
	"testing"
)

var errCustomPolicyForTest = errors.New("custom policy")

// TestCaptureRestoreFullSnapshot verifies Capture()/Restore() round-trips
// every field the doc comment promises: per-endpoint URL/Enabled, offline
// mode, the custom Policy, and the data directory.
func TestCaptureRestoreFullSnapshot(t *testing.T) {
	t.Cleanup(Reset)
	t.Cleanup(func() { SetDataDir("") })

	// SetDataDir (not SetDataDirPath) stores the given string verbatim, no
	// path resolution — the exact round trip this test wants to pin,
	// distinct from TestDataDirEnvOverride's real path-resolution check
	// below.
	SetDataDir("before")

	snap := Capture()

	// Mutate everything the snapshot should restore.
	SetDataDir("after")

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
// clobber what a broader scope (e.g. a package TestMain) set for a different
// endpoint.
func TestCaptureSubsetLeavesOtherEndpointsAlone(t *testing.T) {
	t.Cleanup(Reset)

	snap := Capture(NAIFSPK)

	Disable(NAIFSPK)
	// A test mistakenly (or legitimately, mid-test) disabling NAIFLSK too —
	// Restore() below must NOT touch it, since it wasn't captured.
	Disable(NAIFLSK)

	snap.Restore()

	if _, err := URL(NAIFSPK); err != nil {
		t.Errorf("Restore() left NAIFSPK as the test changed it: %v", err)
	}

	if _, err := URL(NAIFLSK); !errors.Is(err, ErrEndpointDisabled) {
		t.Errorf("Restore(subset) touched NAIFLSK, which was outside the captured subset: URL = %v, want ErrEndpointDisabled", err)
	}
}

// TestWithScopeRestoresOnPanic verifies WithScope restores its captured
// configuration even when fn panics, via defer.
func TestWithScopeRestoresOnPanic(t *testing.T) {
	t.Cleanup(Reset)

	func() {
		defer func() { _ = recover() }()

		WithScope(func() {
			SetPolicy(func(Endpoint, int64) error { return errCustomPolicyForTest })

			panic("boom")
		})
	}()

	if err := CheckDownload(NAIFSPK, "de440s.bsp", 32<<20); err != nil {
		t.Errorf("CheckDownload after a panicking WithScope = %v, want nil — Restore must run via defer", err)
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
