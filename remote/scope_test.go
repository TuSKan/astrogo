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

	SetDataDirPath("before")
	EnableDownloads(NAIFSPK, 10<<20)

	snap := Capture()

	// Mutate everything the snapshot should restore.
	SetDataDirPath("after")
	DisableDownloads(NAIFSPK)

	if err := SetURL(NAIFSPK, "https://example.invalid/mutated"); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	Disable(NAIFSPK)
	SetOffline(true)

	SetPolicy(func(Endpoint, int64) error { return errCustomPolicyForTest })

	snap.Restore()

	if got := DataDir(); got != "before" {
		t.Errorf("DataDir after Restore = %q, want %q", got, "before")
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

	EnableDownloads(NAIFLSK, 0) // stands in for a TestMain-wide grant

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

	EnableDownloads(NAIFSPK, 5<<20)

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

// TestDataDirEnvOverride exercises DataDir's full precedence chain:
// explicit SetDataDir wins over DataDirEnv, which wins over the OS
// default.
func TestDataDirEnvOverride(t *testing.T) {
	t.Cleanup(func() { SetDataDir("") })

	SetDataDir("")
	t.Setenv(DataDirEnv, "/from/env")

	if got := DataDir(); got != "/from/env" {
		t.Errorf("DataDir() with only %s set = %q, want %q", DataDirEnv, got, "/from/env")
	}

	SetDataDirPath("/explicit")

	if got := DataDir(); got != "/explicit" {
		t.Errorf("DataDir() with SetDataDirPath set = %q, want the explicit value to win over %s", got, DataDirEnv)
	}
}
