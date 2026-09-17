package docsguard_test

import (
	"os/exec"
	"sort"
	"strings"
	"testing"
)

// eopLoaderPkg registers the IERS loader with astrogo/time from its init, and
// is the only package in the module that does.
const eopLoaderPkg = "github.com/TuSKan/astrogo/remote/eop"

// TestTheEOPLoaderIsNeverInALibraryClosure is #294, kept from happening twice.
//
// # What went wrong the first time
//
// Between v0.14.0 and v0.19.0 `plan` started importing `remote`, and `remote`
// at the time registered the IERS EOP loader from its own init. So merely
// importing `plan` gave every caller a live loader — a program that had never
// heard of `remote`, never called `EnableDownloads`, and under the documented
// contract should have degraded to zero EOP, instead took a
// filesystem-touching path on every epoch lookup for the life of the process.
// The reporter's suite hung two packages for a full ten-minute timeout with the
// CPU near idle.
//
// The registration has since moved to `remote/eop`, which exports nothing and
// has to be named to be linked. That is the fix. This is what keeps it.
//
// # Why the existing guard is not enough
//
// TestSubpackagesAreNotImportedDirectly already requires that `remote/eop` be
// imported blank and never for a symbol. It says nothing about *who* may import
// it — and #294 was not a package reaching for a symbol, it was a library
// package dragging the loader into everybody's binary without naming it. A
// blank import in `plan` would satisfy that test and reintroduce this exactly.
//
// So the rule here is about the closure rather than the import: no library
// package may have the loader anywhere in its transitive imports. Deciding to
// fetch Earth orientation data belongs to the program, which is the only layer
// that knows whether talking to the IERS is acceptable.
//
// # What may import it
//
// Tests and commands. Both are the top of their own binary and neither is
// linked into anybody else's, so naming the loader there commits nobody.
//
// Test-only imports are invisible to this check, because it reads the import
// list `go list` reports for the package itself rather than the separate one it
// reports for that package's tests. That distinction is what makes
// `time/eop_test.go` and the validation suite legitimate while a blank import
// in `plan` is not.
func TestTheEOPLoaderIsNeverInALibraryClosure(t *testing.T) {
	t.Parallel()

	// One `go list` for the whole module: import path, whether it is a command,
	// and its non-test imports. Building the graph here and walking it is far
	// cheaper than a `go list -deps` per package.
	cmd := exec.CommandContext(t.Context(), "go", "list",
		"-f", "{{.ImportPath}}\t{{.Name}}\t{{join .Imports \",\"}}", "./...",
	)

	// From the module root, not this package's directory: a test's working
	// directory is its own package, where ./... lists one package and this
	// guard would pass by checking nothing.
	cmd.Dir = repoRoot(t)

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}

	imports := map[string][]string{}
	isCommand := map[string]bool{}

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		field := strings.Split(strings.TrimSpace(line), "\t")
		if len(field) != 3 {
			continue
		}

		path, name, deps := field[0], field[1], field[2]

		isCommand[path] = name == "main"

		if deps != "" {
			imports[path] = strings.Split(deps, ",")
		} else {
			imports[path] = nil
		}
	}

	if len(imports) == 0 {
		t.Fatal("go list returned no packages; this guard is not checking anything")
	}

	var offenders []string

	for path := range imports {
		// The loader may import itself into its own closure, and a command is
		// the top of its own binary.
		if path == eopLoaderPkg || isCommand[path] {
			continue
		}

		if via := pathToEOPLoader(imports, path); via != nil {
			offenders = append(offenders, strings.Join(via, " -> "))
		}
	}

	if len(offenders) > 0 {
		sort.Strings(offenders)

		t.Errorf("%d library package(s) pull the IERS EOP loader into every consumer's binary.\n\n%s\n\n"+
			"Registering the loader is the program's decision, not a library's: it is what turns an\n"+
			"epoch lookup into a filesystem-touching path for callers who never asked for Earth\n"+
			"orientation data at all. That is #294, which hung a reporter's test suite for ten\n"+
			"minutes a package.\n\n"+
			"A test or a command may import %s. A package anything else links may not.",
			len(offenders), strings.Join(offenders, "\n"), eopLoaderPkg)
	}
}

// pathToEOPLoader returns the import chain from start to the loader, or nil if
// there is none. The chain rather than a bool, because "plan imports it" and
// "plan imports something that imports it" need different fixes and the
// difference is invisible from the answer alone.
func pathToEOPLoader(imports map[string][]string, start string) []string {
	type step struct {
		pkg   string
		chain []string
	}

	seen := map[string]bool{start: true}
	queue := []step{{pkg: start, chain: []string{start}}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		for _, dep := range imports[cur.pkg] {
			if dep == eopLoaderPkg {
				return append(append([]string{}, cur.chain...), dep)
			}

			// Only packages go list reported, which is this module's own —
			// the loader cannot hide behind a dependency outside it.
			if _, ours := imports[dep]; !ours || seen[dep] {
				continue
			}

			seen[dep] = true
			queue = append(queue, step{pkg: dep, chain: append(append([]string{}, cur.chain...), dep)})
		}
	}

	return nil
}
