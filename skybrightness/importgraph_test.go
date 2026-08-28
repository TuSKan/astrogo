package skybrightness_test

import (
	"go/build"
	"strings"
	"testing"
)

const modulePath = "github.com/TuSKan/astrogo"

// forbidden lists packages the sky-radiance core must not import directly,
// with the reason, so a failure explains itself.
//
//nolint:gochecknoglobals // test fixture table
var forbidden = map[string]string{
	modulePath + "/remote":      "the core resolves no data; a provider layer hands it in already resolved",
	modulePath + "/remote/file": "the core touches no storage",
	modulePath + "/remote/api":  "the core makes no network calls",
	modulePath + "/fits":        "the core carries no file-format dependency",
	modulePath + "/plan":        "plan is orchestration and sits above this package",
	"net/http":                  "the core makes no network calls",
	"os":                        "the core touches no filesystem",
	"github.com/scigolib/hdf5":  "the core carries no heavy dataset dependency",
}

// The core is a pure numeric package: it computes radiance from a Scene it
// was handed, and resolves nothing itself. Datasets are fetched by a
// provider layer, which will live under skybrightness/dataset/... and is
// the only tier permitted these imports.
//
// This checks DIRECT imports. A transitive ban would be wrong rather than
// stricter: the spec requires reusing coord, ephemeris and time, and those
// legitimately reach remote for Earth-orientation data and JPL kernels,
// both consent-gated. What the core must not do is resolve data itself.
// That evaluation genuinely performs no network access is asserted
// behaviourally by TestEstimateWorksOffline, which is the real guarantee.
func TestCoreDoesNotImportIOPackages(t *testing.T) {
	t.Parallel()

	pkg, err := build.Import(modulePath+"/skybrightness", "", 0)
	if err != nil {
		t.Fatalf("import skybrightness: %v", err)
	}

	for _, imp := range pkg.Imports {
		if reason, bad := forbidden[imp]; bad {
			t.Errorf("skybrightness imports %q (%s)", imp, reason)
		}
	}
}

// Every astrogo package the core imports must itself be a layer at or
// below it, so the dependency direction stays one-way.
func TestCoreImportsOnlyLowerLayers(t *testing.T) {
	t.Parallel()

	allowed := map[string]bool{
		modulePath + "/angle":          true,
		modulePath + "/atmosphere":     true,
		modulePath + "/constants":      true,
		modulePath + "/coord":          true,
		modulePath + "/ephemeris":      true,
		modulePath + "/ephemeris/core": true,
		modulePath + "/magnitude":      true,
		modulePath + "/optics":         true,
		modulePath + "/time":           true,
		modulePath + "/unit":           true,
		modulePath + "/vector":         true,
	}

	pkg, err := build.Import(modulePath+"/skybrightness", "", 0)
	if err != nil {
		t.Fatalf("import skybrightness: %v", err)
	}

	for _, imp := range pkg.Imports {
		if !strings.HasPrefix(imp, modulePath) {
			continue // standard library
		}

		if !allowed[imp] {
			t.Errorf("skybrightness imports %q, which is not a declared lower layer", imp)
		}
	}
}
