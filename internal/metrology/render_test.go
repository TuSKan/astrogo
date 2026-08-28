package metrology_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/metrology"
)

func renderResults() []metrology.Result {
	return []metrology.Result{
		{
			Suite:     "zzz.last.alphabetically",
			Status:    metrology.StatusVerified,
			Generated: "2026-08-28T12:00:00Z",
			Commit:    "3536d3cd8846848375a7d9694948f232306d22a7",
			Reference: metrology.Reference{
				Kind: metrology.KindHorizons, Name: "JPL Horizons", Version: "DE441",
			},
			Contract: testContract(3.0),
			Stats:    metrology.Stats{N: 103, P50: 0.4329, P95: 1.902, P99: 1.991, Max: 2.071},
		},
		{
			Suite:     "aaa.first.alphabetically",
			Status:    metrology.StatusVerified,
			Generated: "2026-08-28T12:00:00Z",
			Commit:    "abcdef1234567890",
			Reference: metrology.Reference{
				Kind: metrology.KindSOFA, Name: "gofa", Version: "v1.19.1", SharedAncestor: "SOFA",
			},
			Contract: testContract(1.0),
			Stats:    metrology.Stats{N: 6, P50: 0.2, P95: 0.27, P99: 0.28, Max: 0.282},
		},
	}
}

// Rows are ordered by suite name so a regenerated document diffs only where a
// number moved, never because a map iterated differently.
func TestRenderMarkdownIsStablyOrdered(t *testing.T) {
	t.Parallel()

	got := metrology.RenderMarkdown(renderResults())

	first := strings.Index(got, "aaa.first")
	second := strings.Index(got, "zzz.last")

	if first < 0 || second < 0 {
		t.Fatalf("both suites should appear:\n%s", got)
	}

	if first > second {
		t.Error("rows are not sorted by suite name, so the table will churn between runs")
	}
}

// The independence column is the one that keeps the table honest.
func TestRenderMarkdownDisclosesSharedAncestry(t *testing.T) {
	t.Parallel()

	got := metrology.RenderMarkdown(renderResults())

	if !strings.Contains(got, "shares SOFA — consistency check") {
		t.Errorf("a SOFA-derived reference is not labelled as a consistency check:\n%s", got)
	}

	if !strings.Contains(got, "independent") {
		t.Errorf("a reference with no shared ancestry is not labelled independent:\n%s", got)
	}
}

// A suite that did not run must not print zeroes.
//
// Its Stats are the zero value, and a zero in an error column reads as
// perfect agreement — the strongest possible claim, made by a suite that
// measured nothing.
func TestRenderMarkdownBlanksUnverifiedNumbers(t *testing.T) {
	t.Parallel()

	got := metrology.RenderMarkdown([]metrology.Result{{
		Suite:     "unreachable.suite",
		Status:    metrology.StatusNotVerified,
		Reason:    "JPL Horizons is unreachable",
		Generated: "2026-08-28T12:00:00Z",
		Commit:    "abcdef1234567890",
		Contract:  testContract(3.0),
	}})

	row := ""

	for line := range strings.SplitSeq(got, "\n") {
		if strings.Contains(line, "unreachable.suite") {
			row = line
		}
	}

	if row == "" {
		t.Fatalf("the unverified suite is missing from the table entirely — being absent is the one\n"+
			"rendering that reads as though the suite never existed:\n%s", got)
	}

	if strings.Contains(row, "| 0 |") || strings.Contains(row, "| 0.0") {
		t.Errorf("a suite that did not run printed a zero measurement: %s", row)
	}

	for _, want := range []string{"NOT VERIFIED", "unreachable"} {
		if !strings.Contains(row, want) {
			t.Errorf("row is missing %q: %s", want, row)
		}
	}
}

// Every row is stamped, because a table of accuracy figures says nothing
// about whether it is current.
func TestRenderMarkdownStampsEachRow(t *testing.T) {
	t.Parallel()

	got := metrology.RenderMarkdown(renderResults())

	if !strings.Contains(got, "2026-08-28") {
		t.Errorf("no verification date in the table:\n%s", got)
	}

	if !strings.Contains(got, "3536d3cd") {
		t.Errorf("no commit in the table:\n%s", got)
	}

	// Abbreviated, not the full 40 characters, which would dominate the row.
	if strings.Contains(got, "3536d3cd8846848375a7d9694948f232306d22a7") {
		t.Error("the full commit hash is printed; it should be abbreviated")
	}
}

// The table has to state that its numbers are measurements and not bounds,
// because that distinction is the entire point and a reader arriving at the
// table alone has no other way to know.
func TestRenderMarkdownSeparatesMeasurementFromContract(t *testing.T) {
	t.Parallel()

	got := metrology.RenderMarkdown(renderResults())

	for _, want := range []string{"measured", "not a bound", "Contract"} {
		if !strings.Contains(got, want) {
			t.Errorf("the table does not distinguish measurement from contract (%q missing):\n%s", want, got)
		}
	}
}

func TestUpdateRegionReplacesOnlyTheMarkedPart(t *testing.T) {
	t.Parallel()

	doc := "# Validation\n\nProse above that must survive.\n\n" +
		metrology.BeginMarker + "\n\nold table\n\n" + metrology.EndMarker +
		"\n\nProse below that must also survive.\n"

	got, err := metrology.UpdateRegion(doc, "new table\n")
	if err != nil {
		t.Fatalf("UpdateRegion: %v", err)
	}

	for _, want := range []string{
		"Prose above that must survive.",
		"Prose below that must also survive.",
		"new table",
		metrology.BeginMarker,
		metrology.EndMarker,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("result is missing %q:\n%s", want, got)
		}
	}

	if strings.Contains(got, "old table") {
		t.Errorf("the previous table survived:\n%s", got)
	}

	// Idempotent: rendering the same body twice must not accumulate markers
	// or blank lines, or every regeneration would show a diff.
	again, err := metrology.UpdateRegion(got, "new table\n")
	if err != nil {
		t.Fatalf("second UpdateRegion: %v", err)
	}

	if again != got {
		t.Errorf("UpdateRegion is not idempotent:\n--- first ---\n%s\n--- second ---\n%s", got, again)
	}
}

// Appending to a document with no markers would leave the stale table in
// place beside the new one, and both would look current.
func TestUpdateRegionRefusesADocumentWithoutMarkers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		doc  string
		want error
	}{
		{"no markers at all", "# Validation\n\njust prose\n", metrology.ErrNoMarkers},
		{"begin only", "# V\n" + metrology.BeginMarker + "\n", metrology.ErrNoMarkers},
		{"end only", "# V\n" + metrology.EndMarker + "\n", metrology.ErrNoMarkers},
		{"reversed", "# V\n" + metrology.EndMarker + "\nx\n" + metrology.BeginMarker + "\n", metrology.ErrMarkerOrder},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := metrology.UpdateRegion(tc.doc, "table")
			if !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
		})
	}
}
