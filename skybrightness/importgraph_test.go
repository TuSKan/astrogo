package skybrightness_test

import (
	"go/build"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

const modulePath = "github.com/TuSKan/astrogo"

// forbidden lists packages the sky-radiance core must not reach, with the
// reason, so a failure explains itself.
var forbidden = map[string]string{
	modulePath + "/remote":      "the core resolves no data; a provider layer hands it in already resolved",
	modulePath + "/remote/file": "the core touches no storage",
	modulePath + "/remote/api":  "the core makes no network calls",
	modulePath + "/fits":        "the core carries no file-format dependency",
	modulePath + "/plan":        "plan is orchestration and sits above this package",
	"net":                       "the core makes no network calls",
	"net/http":                  "the core makes no network calls",
	"os":                        "the core touches no filesystem",
	"github.com/scigolib/hdf5":  "the core carries no heavy dataset dependency",
}

// directOnly are the forbidden packages the standard library brings in
// itself (fmt imports os), so only a direct import of one is the core's own
// choice. Every other forbidden package is banned from the whole closure.
var directOnly = map[string]bool{"os": true}

// The core is a pure numeric package: it computes radiance from a Scene it
// was handed, and resolves nothing itself. Datasets are fetched by a
// provider layer under skybrightness/dataset/..., the only tier permitted
// these imports.
//
// This checks the whole import closure, not only direct imports (#973). It
// used to check direct imports alone, on the premise that coord, ephemeris
// and time reach remote for Earth-orientation data and JPL kernels. That
// stopped being true when time began taking EOP bytes through a loader that
// remote/eop registers (2e07488c), and JPL kernels live in ephemeris/jpl,
// which the core does not import. So nothing the core imports reaches remote,
// and a new import of a package that fetches is caught however deep it sits.
// At runtime the core reaches outside only through what its caller hands it:
// the Scene's ephemeris provider, and an EOP loader registered by a blank
// import of remote/eop. TestEstimateWorksOffline holds the real components
// to that.
func TestCoreDoesNotImportIOPackages(t *testing.T) {
	t.Parallel()

	closure := importClosure(t, modulePath+"/skybrightness")

	// The walk must reach past the core, or a forbidden package deep in the
	// graph could not be found at all.
	for _, want := range []string{modulePath + "/coord", modulePath + "/time", "fmt"} {
		if _, ok := closure[want]; !ok {
			t.Fatalf("the import closure has no %s; the walk is not following imports", want)
		}
	}

	for path, chain := range closure {
		reason, bad := forbidden[path]
		if !bad || (directOnly[path] && len(chain) > 2) {
			continue
		}

		t.Errorf("skybrightness reaches %q (%s), through %s", path, reason, strings.Join(chain, " -> "))
	}
}

// importClosure returns every package that root links, directly or not, each
// with the shortest import chain from root that reaches it. It asks the
// toolchain, as internal/docsguard's closure guards do, so build constraints
// and the standard library's vendored packages resolve exactly as a build
// resolves them. Test files are not followed, only what a build of root links.
func importClosure(t *testing.T, root string) map[string][]string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps",
		"-f", "{{.ImportPath}}\t{{join .Imports \",\"}}", root).Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", root, err)
	}

	imports := make(map[string][]string)

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		path, list, _ := strings.Cut(strings.TrimSpace(line), "\t")
		if list != "" {
			imports[path] = strings.Split(list, ",")
		} else {
			imports[path] = nil
		}
	}

	chains := map[string][]string{root: {root}}
	queue := []string{root}

	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]

		for _, imp := range imports[next] {
			if _, seen := chains[imp]; seen {
				continue
			}

			chains[imp] = append(slices.Clone(chains[next]), imp)
			queue = append(queue, imp)
		}
	}

	return chains
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
