package skybrightness

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestEveryComponentStatesItsProvenance requires every field a provenance
// cannot do without, from every component the package declares (#936).
//
// The components are found by reading the source, not from a list alone. A
// list covers only the components someone remembered to put in it:
// TestFullSkyProvenance checks the six its scene assembles, which is how
// ExtragalacticBackground shipped with no ExpectedAccuracy, being not one of
// them. Here a type that declares a Provenance method is a component, and one
// missing from components below fails the test until it is added.
//
// SecondaryReferences and Datasets stay optional: a component built on one
// paper and no data product has neither. So does Version, which
// [Provenance.String] already omits when empty: four components have never
// needed one.
func TestEveryComponentStatesItsProvenance(t *testing.T) {
	t.Parallel()

	components := []Component{
		&Airglow{},
		&ArtificialSkyglow{},
		&CloudySkyglow{},
		&DiffuseGalacticLight{},
		&ExtragalacticBackground{},
		&IntegratedStarlight{},
		&ScatteredMoonlight{},
		&ZodiacalLight{},
	}

	listed := make(map[string]bool, len(components))

	for _, c := range components {
		name := reflect.TypeOf(c).Elem().Name()
		listed[name] = true

		p := c.Provenance()

		for _, field := range []struct{ name, value string }{
			{"Model", p.Model},
			{"PrimaryReference", p.PrimaryReference},
			{"Equations", p.Equations},
			{"ValidityDomain", p.ValidityDomain},
			{"ExpectedAccuracy", p.ExpectedAccuracy},
		} {
			if strings.TrimSpace(field.value) == "" {
				t.Errorf("%s's provenance states no %s", name, field.name)
			}
		}

		if len(p.KnownApproximations) == 0 {
			t.Errorf("%s's provenance lists no approximations, which is unlikely to be true", name)
		}
	}

	for _, name := range declaredComponents(t) {
		if !listed[name] {
			t.Errorf("%s declares a Provenance method but this test does not check it; "+
				"add it to components", name)
		}
	}
}

// declaredComponents returns the receiver type of every Provenance method in
// the package's own source, test files excluded.
func declaredComponents(t *testing.T) []string {
	t.Helper()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}

	fset := token.NewFileSet()

	var names []string

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Provenance" {
				continue
			}

			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}

			if id, ok := recv.(*ast.Ident); ok {
				names = append(names, id.Name)
			}
		}
	}

	// Finding none means the test is reading the wrong directory, and would
	// otherwise pass while checking nothing.
	if len(names) == 0 {
		t.Fatal("found no Provenance method in the package source")
	}

	return names
}
