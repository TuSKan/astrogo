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
	dir := os.Getenv(metrology.OutDirEnv)
	if dir == "" {
		t.Skipf("%s is not set; run the suites with it pointing at a directory first — "+
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

	if !*updateAccuracy {
		t.Fatalf("%s would change; rerun with -args -update-accuracy to write it.\n"+
			"Collected %d results covering: %s", accuracyDoc, len(results), suiteNames(results))
	}

	if err := os.WriteFile(accuracyDoc, []byte(updated), 0o600); err != nil {
		t.Fatalf("writing %s: %v", accuracyDoc, err)
	}

	t.Logf("wrote %d rows to %s", len(results), accuracyDoc)
}

func suiteNames(results []metrology.Result) string {
	names := make([]string, 0, len(results))
	for _, r := range results {
		names = append(names, r.Suite)
	}

	return strings.Join(names, ", ")
}
