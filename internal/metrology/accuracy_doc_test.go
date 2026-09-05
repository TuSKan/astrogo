//go:build validation

package metrology_test

import (
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/metrology"
)

// updateAccuracy gates every write to the checked-in document, for the same
// reason the corpus generator is gated: a document that some test rewrites on
// its way past a failure is not a record of anything.
var updateAccuracy = flag.Bool("update-accuracy", false,
	"rewrite the generated region of docs/VALIDATION.md from the collected results")

// ambientOutDir is the output directory the operator asked for, captured
// before TestMain takes it away.
//
// Declared here, in the only file that reads it, rather than beside the
// TestMain that clears it. Package-level variables are initialised before
// TestMain is called, so this still sees the real value — and putting it in
// the untagged file would leave a symbol with no consumer in a build that
// compiles neither tag, which is what "golangci-lint run" does and what it
// reports as unused.
var ambientOutDir = os.Getenv(metrology.OutDirEnv)

// accuracyDoc is the document whose marked region is generated.
var accuracyDoc = filepath.Join("..", "..", "docs", "VALIDATION.md")

// TestRenderAccuracyReport collects every suite's result and renders the
// evidence table into docs/VALIDATION.md.
//
// # Why this is a test and not a command
//
// Because the results it reads are produced by tests, and a separate binary
// would need its own way to find them, its own flag parsing and its own place
// in the tree — for a job that is three file operations. astrogo has no cmd/
// directory and does not need one for this.
//
// # How to run it
//
// The suites write their results only when ASTROGO_METROLOGY_OUT names a
// directory, so an ordinary run leaves nothing behind. To refresh the table:
//
//	export ASTROGO_METROLOGY_OUT=/tmp/astrogo-validation
//	go test -tags=validation ./...
//	go test -tags=network ./...
//	go test -tags=validation -run TestRenderAccuracyReport ./internal/metrology/ -args -update-accuracy
//
// The rendering step is deliberately separate from CI, which uploads the
// results as artifacts and does not commit. A scheduled run happening while
// an external service is down would otherwise rewrite the record with a
// table full of NOT VERIFIED rows — accurate about that run, and a worse
// document than the one it replaced.
func TestRenderAccuracyReport(t *testing.T) {
	// Acts only when asked, and never merely because the environment
	// variable happens to be set.
	//
	// It used to fail whenever the rendered table differed from the file,
	// which sounds like a staleness check and is not one: it cannot tell a
	// document that has fallen behind from a run that collected only some of
	// the suites. "go test -tags=validation ./..." with the variable set —
	// the documented way to collect results — has both properties at once,
	// since this test runs inside the very invocation still producing the
	// documents it reads, in whatever order the packages finish. Every such
	// run failed.
	if !*updateAccuracy {
		t.Skipf("pass -args -update-accuracy to rewrite %s; see this test's doc comment", accuracyDoc)
	}

	// The directory comes from ambientOutDir rather than the environment,
	// which TestMain empties so this package's fixtures cannot write into a
	// real collection. See main_test.go.
	dir := ambientOutDir
	if dir == "" {
		t.Fatalf("%s is not set; run the suites with it pointing at a directory first — "+
			"see this test's doc comment", metrology.OutDirEnv)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	var results []metrology.Result

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Errorf("reading %s: %v", entry.Name(), err)

			continue
		}

		res, err := metrology.ReadResult(data)
		if err != nil {
			t.Errorf("%s: %v", entry.Name(), err)

			continue
		}

		results = append(results, res)
	}

	if len(results) == 0 {
		t.Fatalf("no results in %s — the suites write there only when %s is set while they run, "+
			"not when this test runs", dir, metrology.OutDirEnv)
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Suite < results[j].Suite })

	for _, r := range results {
		t.Logf("  %-40s %-18s n=%d", r.Suite, r.Status, r.Stats.N)
	}

	doc, err := os.ReadFile(accuracyDoc)
	if err != nil {
		t.Fatalf("reading %s: %v", accuracyDoc, err)
	}

	updated, err := metrology.UpdateRegion(string(doc), metrology.RenderMarkdown(results))
	if err != nil {
		t.Fatalf("%v", err)
	}

	if updated == string(doc) {
		t.Logf("%s is already current for these %d results", accuracyDoc, len(results))

		return
	}

	if err := os.WriteFile(accuracyDoc, []byte(updated), 0o600); err != nil {
		t.Fatalf("writing %s: %v", accuracyDoc, err)
	}

	t.Logf("wrote %d rows to %s", len(results), accuracyDoc)
}
