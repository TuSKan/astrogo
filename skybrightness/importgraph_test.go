package skybrightness_test

import (
	"go/build"
	"strings"
	"testing"
)

// TestCoreDoesNotImportAtlas enforces the layering rule that the core
// skybrightness package (numeric, pure, no IO) must never depend on its atlas
// sibling (file decoding, GeoTIFF). The atlas package depends on core, not the
// other way around, so a violation would create a cycle in spirit and pull
// decode/IO concerns into the hot numeric path.
func TestCoreDoesNotImportAtlas(t *testing.T) {
	t.Parallel()

	const (
		corePkg  = "github.com/TuSKan/astrogo/skybrightness"
		atlasPkg = "github.com/TuSKan/astrogo/skybrightness/atlas"
	)

	pkg, err := build.Default.Import(corePkg, "", 0)
	if err != nil {
		t.Fatalf("import %s: %v", corePkg, err)
	}

	// Both production and in-package test imports must stay clear of atlas.
	all := append([]string{}, pkg.Imports...)
	all = append(all, pkg.TestImports...)

	for _, imp := range all {
		if imp == atlasPkg || strings.HasPrefix(imp, atlasPkg+"/") {
			t.Errorf("core skybrightness must not import %s (found %q)", atlasPkg, imp)
		}
	}
}
