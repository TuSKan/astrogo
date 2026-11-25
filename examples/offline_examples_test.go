package examples_test

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/remote"
)

// offlineRow matches a row of the README's examples table whose Network
// column says the example needs none:
//
//	| [`01_where_is_mars`](examples/01_where_is_mars/) | ... | none |
var offlineRow = regexp.MustCompile("(?m)^\\| \\[`([0-9]{2}_[a-z0-9_]+)`\\]\\(examples/[^)]+\\) \\|.*\\| none \\|\\s*$")

// networkCalls are the calls that reach the network: an endpoint override, a
// catalog resolver or provider, and a kernel-backed ephemeris provider, which
// downloads its kernel on first use. A program that turns offline mode on
// with remote.SetOffline reads only the cache, so in one that does, a kernel
// provider does not count — that is what 19_offline_setup demonstrates.
var networkCalls = map[string]bool{
	"remote.SetURL":       true,
	"catalog.NewResolver": true,
	"catalog.NewProvider": true,
	"eph.NewProvider":     true,
}

// offlineOnlyCalls are the networkCalls that read only the cache once the
// program has called remote.SetOffline.
var offlineOnlyCalls = map[string]bool{"eph.NewProvider": true}

// networkCallIn returns the first call in the Go source file at path that
// networkCalls names, as "pkg.Func", or "" if it makes none. It reads the
// syntax tree, so a call quoted in a string or a comment does not count.
func networkCallIn(t *testing.T, path string) string {
	t.Helper()

	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var calls []string

	offline := false

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}

		name := pkg.Name + "." + sel.Sel.Name
		if name == "remote.SetOffline" {
			offline = true
		}

		if networkCalls[name] {
			calls = append(calls, name)
		}

		return true
	})

	for _, c := range calls {
		if offline && offlineOnlyCalls[c] {
			continue
		}

		return c
	}

	return ""
}

// offlineExamples returns the examples the README says need no network.
func offlineExamples(t *testing.T) []string {
	t.Helper()

	readme, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	rows := offlineRow.FindAllStringSubmatch(string(readme), -1)

	names := make([]string, 0, len(rows))
	for _, m := range rows {
		names = append(names, m[1])
	}

	// A table that parses to almost nothing means the pattern stopped
	// matching the README, not that every example went online.
	if len(names) < 8 {
		t.Fatalf("found %d offline examples in the README; the table no longer matches offlineRow", len(names))
	}

	return names
}

// TestOfflineExamplesMakeNoNetworkCall holds the README's Network column to
// the code: an example it lists as needing none makes none of the calls that
// reach the network. 02 and 03 were listed as offline while resolving their
// targets through SIMBAD, and failed during the CDS outage of 2026-11-13.
func TestOfflineExamplesMakeNoNetworkCall(t *testing.T) {
	t.Parallel()

	for _, name := range offlineExamples(t) {
		if call := networkCallIn(t, filepath.Join(name, "main.go")); call != "" {
			t.Errorf("%s is listed as needing no network, but calls %s", name, call)
		}
	}
}

// TestOfflineExamplesRun runs every example the README lists as needing no
// network, against a fresh cache, and requires each to exit 0. Building them
// was all CI did before, and 14 panicked on every run for as long as anyone
// can tell.
//
// None may print a warning but one: an example that needs Earth orientation
// downloads the IERS bulletin into its empty cache, and every download is
// logged at Warn. Any other warning is the example's own.
func TestOfflineExamplesRun(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode: running every offline example takes a few minutes")
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go not on PATH: %v", err)
	}

	// A fresh, empty cache, so nothing downloaded earlier can stand in for a
	// network call. ASTROGO_CACHE_DIR holds a file URL: "file:///tmp/x" on
	// Unix, "file:///C:/Users/x" on Windows.
	cache := filepath.ToSlash(t.TempDir())
	if !strings.HasPrefix(cache, "/") {
		cache = "/" + cache
	}

	for _, name := range offlineExamples(t) {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()

			cmd := exec.CommandContext(ctx, goBin, "run", "./"+name)

			cmd.Env = append(os.Environ(), "ASTROGO_CACHE_DIR=file://"+cache, "NO_COLOR=1")

			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s failed: %v\n%s", name, err, out)
			}

			// Earth orientation loads on first need, so an example computing a
			// topocentric place downloads the IERS bulletin into its empty
			// cache, and every download is logged at Warn. Any other warning
			// is the example's own.
			iers := "endpoint=" + string(remote.IERSFinals2000A) + " "

			for line := range strings.SplitSeq(string(out), "\n") {
				if strings.Contains(line, "level=WARN") && !strings.Contains(line, iers) {
					t.Errorf("%s warned:\n%s", name, out)

					break
				}
			}
		})
	}
}
