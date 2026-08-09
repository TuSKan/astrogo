package passband

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness"
)

// writeSyntheticBundle builds a minimal, valid bundle directory (one
// top-hat curve, no Vega zero point) in t.TempDir() — no network, no
// real published data, matching this codebase's "never fabricate
// published data" rule by using an obviously-synthetic response shape
// rather than claiming to reproduce a real filter.
func writeSyntheticBundle(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	manifestJSON := `{
  "version": "test-v0",
  "curves": [
    {"id": "test.tophat", "file": "tophat.csv", "system": "AB", "detector": "PhotonCounting",
     "checksum": "sha256:test", "source": "synthetic test fixture", "licence": "n/a"}
  ]
}`

	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifestJSON), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	curveCSV := "wavelength_nm,response\n500,1.0\n600,1.0\n"
	if err := os.WriteFile(filepath.Join(dir, "tophat.csv"), []byte(curveCSV), 0o600); err != nil {
		t.Fatalf("write curve: %v", err)
	}

	return dir
}

func TestOpenBundle_RoundTrip(t *testing.T) {
	dir := writeSyntheticBundle(t)

	set, err := OpenBundle(dir)
	if err != nil {
		t.Fatalf("OpenBundle: %v", err)
	}

	if set.Version() != "test-v0" {
		t.Errorf("Version() = %q, want %q", set.Version(), "test-v0")
	}

	ids := set.List()
	if len(ids) != 1 || ids[0] != "test.tophat" {
		t.Fatalf("List() = %v, want [test.tophat]", ids)
	}

	pb, err := set.Get("test.tophat")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if pb.System != skybrightness.SystemAB {
		t.Errorf("System = %v, want SystemAB", pb.System)
	}

	lo, hi := pb.Range()
	if lo != 500 || hi != 600 {
		t.Errorf("Range() = (%v, %v), want (500, 600)", lo, hi)
	}

	if pb.VegaZP != nil {
		t.Error("expected nil VegaZP for a curve with no vega_mean_flambda entry")
	}
}

func TestOpenBundle_MissingManifestErrors(t *testing.T) {
	dir := t.TempDir()

	if _, err := OpenBundle(dir); err == nil {
		t.Error("expected an error for a directory with no manifest.json, got nil")
	}
}

func TestOpenBundle_UnknownGet(t *testing.T) {
	dir := writeSyntheticBundle(t)

	set, err := OpenBundle(dir)
	if err != nil {
		t.Fatalf("OpenBundle: %v", err)
	}

	if _, err := set.Get("does.not.exist"); err == nil {
		t.Error("expected ErrPassbandNotFound for an unknown ID, got nil")
	}
}
