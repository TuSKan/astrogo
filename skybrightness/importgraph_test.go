package skybrightness_test

import (
	"go/build"
	"strings"
	"testing"
)

// TestCoreDoesNotImportSiblings enforces the layering rule that the core
// skybrightness package (numeric, pure, no IO) must never depend on its
// atlas or lpmap siblings (file decoding / live HTTP client respectively).
// Both siblings depend on core, not the other way around, so a violation
// would create a cycle in spirit and pull decode/IO/network concerns into
// the hot numeric path.
func TestCoreDoesNotImportSiblings(t *testing.T) {
	t.Parallel()

	const corePkg = "github.com/TuSKan/astrogo/skybrightness"

	siblings := []string{
		"github.com/TuSKan/astrogo/skybrightness/atlas",
		"github.com/TuSKan/astrogo/skybrightness/lpmap",
	}

	pkg, err := build.Default.Import(corePkg, "", 0)
	if err != nil {
		t.Fatalf("import %s: %v", corePkg, err)
	}

	// Both production and in-package test imports must stay clear of the siblings.
	all := append([]string{}, pkg.Imports...)
	all = append(all, pkg.TestImports...)

	for _, imp := range all {
		for _, sibling := range siblings {
			if imp == sibling || strings.HasPrefix(imp, sibling+"/") {
				t.Errorf("core skybrightness must not import %s (found %q)", sibling, imp)
			}
		}
	}
}
