package skybrightness_test

import (
	"go/build"
	"strings"
	"testing"
)

const corePkg = "github.com/TuSKan/astrogo/skybrightness"

// coreAllowedImports is docs/skybrightness.md §4 rule 1, verbatim: core
// skybrightness may import only stdlib, angle, vector, unit, constants,
// time, coord, ephemeris, atmosphere, internal/parallel. Extend this list
// only alongside a matching update to docs/skybrightness.md §4.
var coreAllowedImports = map[string]bool{
	"github.com/TuSKan/astrogo/angle":             true,
	"github.com/TuSKan/astrogo/vector":            true,
	"github.com/TuSKan/astrogo/unit":              true,
	"github.com/TuSKan/astrogo/constants":         true,
	"github.com/TuSKan/astrogo/time":              true,
	"github.com/TuSKan/astrogo/coord":             true,
	"github.com/TuSKan/astrogo/ephemeris":         true,
	"github.com/TuSKan/astrogo/atmosphere":        true,
	"github.com/TuSKan/astrogo/internal/parallel": true,
}

// TestCoreImportsOnlyAllowedPackages enforces rule 1: every non-stdlib
// import of core skybrightness must be in coreAllowedImports.
func TestCoreImportsOnlyAllowedPackages(t *testing.T) {
	t.Parallel()

	pkg, err := build.Default.Import(corePkg, "", 0)
	if err != nil {
		t.Fatalf("import %s: %v", corePkg, err)
	}

	for _, imp := range pkg.Imports {
		if !strings.Contains(imp, ".") { // stdlib import (no dot in the first path element's domain)
			continue
		}

		if !coreAllowedImports[imp] {
			t.Errorf("core skybrightness imports %q, not in the allowed list (docs/skybrightness.md §4 rule 1)", imp)
		}
	}
}

// TestCoreDoesNotImportSiblings enforces rules 2/3: core must never
// import any of its own siblings (the pure physics packages, or the IO
// tier under dataset/ and lpmap) — siblings depend on core, never the
// reverse.
func TestCoreDoesNotImportSiblings(t *testing.T) {
	t.Parallel()

	siblings := []string{
		corePkg + "/natural",
		corePkg + "/atmos",
		corePkg + "/artificial",
		corePkg + "/rt",
		corePkg + "/surrogate",
		corePkg + "/calib",
		corePkg + "/dataset",
		corePkg + "/lpmap",
	}

	pkg, err := build.Default.Import(corePkg, "", 0)
	if err != nil {
		t.Fatalf("import %s: %v", corePkg, err)
	}

	all := append(append([]string{}, pkg.Imports...), pkg.TestImports...)

	for _, imp := range all {
		for _, sibling := range siblings {
			if imp == sibling || strings.HasPrefix(imp, sibling+"/") {
				t.Errorf("core skybrightness must not import %s (found %q)", sibling, imp)
			}
		}
	}
}

// ioOnlyImports are only ever legitimate from the IO tier (rule 3).
var ioOnlyImports = []string{
	"net/http",
	"github.com/TuSKan/astrogo/remote",
	"github.com/TuSKan/astrogo/fits",
	"github.com/scigolib/hdf5",
}

// TestPureSiblingsDoNotImportIO enforces rule 3 for the pure-physics
// siblings that exist today: natural and atmos must never import remote,
// fits, net/http, or the HDF5 library — only skybrightness/dataset/... and
// skybrightness/lpmap may.
func TestPureSiblingsDoNotImportIO(t *testing.T) {
	t.Parallel()

	pure := []string{corePkg + "/natural", corePkg + "/atmos"}

	for _, p := range pure {
		t.Run(p, func(t *testing.T) {
			t.Parallel()

			pkg, err := build.Default.Import(p, "", 0)
			if err != nil {
				t.Fatalf("import %s: %v", p, err)
			}

			for _, imp := range append(append([]string{}, pkg.Imports...), pkg.TestImports...) {
				for _, forbidden := range ioOnlyImports {
					if imp == forbidden {
						t.Errorf("%s must not import %s (IO tier only)", p, forbidden)
					}
				}

				if strings.HasPrefix(imp, corePkg+"/dataset") || imp == corePkg+"/lpmap" {
					t.Errorf("%s must not import the IO tier %q", p, imp)
				}
			}
		})
	}
}

// TestPlanImportsOnlyCoreSkybrightness enforces rule 4: plan may import
// core skybrightness only, never any of its subpackages. This preserves
// the CLAUDE.md rule that an HDF5-scale dependency never reaches a plan
// user's build — engines are assembled by the application and injected.
func TestPlanImportsOnlyCoreSkybrightness(t *testing.T) {
	t.Parallel()

	pkg, err := build.Default.Import("github.com/TuSKan/astrogo/plan", "", 0)
	if err != nil {
		t.Fatalf("import plan: %v", err)
	}

	for _, imp := range append(append([]string{}, pkg.Imports...), pkg.TestImports...) {
		if imp == corePkg {
			continue
		}

		if strings.HasPrefix(imp, corePkg+"/") {
			t.Errorf("plan must import only %s, not %q", corePkg, imp)
		}
	}
}
