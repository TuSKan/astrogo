package metrology

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Markers delimiting the generated region of a document.
//
// A marked region rather than a whole generated file, because most of what
// astrogo's validation document contains cannot be generated and should not
// be: the refuted hypotheses, the known-limitation prose, the reasons a
// number is what it is. Those are the parts worth reading. What a generator
// can supply is the evidence table underneath them, and it should replace
// exactly that and nothing else.
const (
	BeginMarker = "<!-- BEGIN GENERATED ACCURACY — do not edit by hand -->"
	EndMarker   = "<!-- END GENERATED ACCURACY -->"
)

// Errors from rewriting a document's generated region.
var (
	// ErrNoMarkers marks a document with no generated region to replace.
	ErrNoMarkers = errors.New("metrology: document has no generated-accuracy region")

	// ErrMarkerOrder marks markers that are present but the wrong way round.
	ErrMarkerOrder = errors.New("metrology: end marker precedes begin marker")
)

// RenderMarkdown renders results as the table that goes between the markers.
//
// Sorted by suite name so the table is stable: a regenerated document should
// diff only where a number moved, never because a map iterated differently.
func RenderMarkdown(results []Result) string {
	sorted := append([]Result(nil), results...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Suite < sorted[j].Suite })

	var b strings.Builder

	b.WriteString("| Suite | Reference | Independence | N | p50 | p95 | p99 | Max | Contract | Status | Last verified |\n")
	b.WriteString("|---|---|---|---:|---:|---:|---:|---:|---:|---|---|\n")

	for _, r := range sorted {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			r.Suite,
			referenceCell(r.Reference),
			independenceCell(r.Reference),
			countCell(r),
			statCell(r, r.Stats.P50),
			statCell(r, r.Stats.P95),
			statCell(r, r.Stats.P99),
			statCell(r, r.Stats.Max),
			fmt.Sprintf("%.3g %s", r.Contract.Max, r.Contract.Units),
			statusCell(r),
			verifiedCell(r),
		)
	}

	b.WriteString("\nEvery figure above is a **measured** value over the corpus named in its suite, " +
		"not a bound. The contract column is the bound, and it is a separate claim: it says what " +
		"the software must achieve and why, and it does not move when a measurement does. " +
		"See `internal/metrology` for the reasoning, and each suite's own doc comment for the " +
		"rationale behind its contract.\n")

	return b.String()
}

// referenceCell names what a suite was compared against.
func referenceCell(r Reference) string {
	s := r.Name
	if s == "" {
		s = string(r.Kind)
	}

	if r.Version != "" {
		s += " " + r.Version
	}

	return s
}

// independenceCell is the shared-ancestry disclosure, rendered so a reader
// does not have to already know it.
//
// This is the column that keeps the table honest. astrogo reaches IAU
// reductions through gofa, which is SOFA-derived; a comparison against SOFA
// is therefore a check on the translation, not on the physics, and a row that
// does not say so is claiming more than it has.
func independenceCell(r Reference) string {
	if r.Independent() {
		return "independent"
	}

	return "shares " + r.SharedAncestor + " — consistency check"
}

// countCell and statCell blank out numbers for a suite that did not run,
// rather than printing the zeroes its empty statistics contain. A zero in an
// error column reads as perfect agreement, which is the opposite of what a
// missing measurement means.
func countCell(r Result) string {
	if r.Status == StatusNotVerified {
		return "—"
	}

	return strconv.Itoa(r.Stats.N)
}

func statCell(r Result, v float64) string {
	if r.Status == StatusNotVerified {
		return "—"
	}

	return fmt.Sprintf("%.3g", v)
}

func statusCell(r Result) string {
	switch r.Status {
	case StatusVerified:
		return "✅ verified"
	case StatusViolated:
		return "❌ contract violated"
	case StatusNotVerified:
		reason := r.Reason
		if reason == "" {
			reason = "did not run"
		}

		return "⚠️ NOT VERIFIED — " + reason
	default:
		return string(r.Status)
	}
}

// verifiedCell stamps the row with when and against which commit.
//
// Without it a table of accuracy figures says nothing about whether it is
// current. A row confirmed before the module it describes was rewritten is
// not a validated row, and the only way to see that is to print the date next
// to the number.
func verifiedCell(r Result) string {
	commit := r.Commit
	if len(commit) > 8 {
		commit = commit[:8]
	}

	if commit == "" {
		commit = "unknown"
	}

	date := r.Generated
	if i := strings.IndexByte(date, 'T'); i > 0 {
		date = date[:i]
	}

	if date == "" {
		return commit
	}

	return date + " · `" + commit + "`"
}

// UpdateRegion replaces the marked region of doc with body.
//
// It refuses a document with no markers rather than appending, because
// appending would silently produce a second table and leave the stale one in
// place — which is worse than not updating at all, since both would look
// current.
func UpdateRegion(doc, body string) (string, error) {
	begin := strings.Index(doc, BeginMarker)
	if begin < 0 {
		return "", fmt.Errorf("%w: expected %q", ErrNoMarkers, BeginMarker)
	}

	end := strings.Index(doc, EndMarker)
	if end < 0 {
		return "", fmt.Errorf("%w: expected %q", ErrNoMarkers, EndMarker)
	}

	if end < begin {
		return "", ErrMarkerOrder
	}

	return doc[:begin] + BeginMarker + "\n\n" + strings.TrimRight(body, "\n") + "\n\n" + doc[end:], nil
}
