package docsguard_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Roadmap statuses this guard understands. Anything else is a deliberate
// one-off label — item 28's "✅ Phases 0–5 — superseded and rewritten" is the
// only one today — and is reported but not judged, since the checkbox
// arithmetic below cannot say what such a label ought to mean.
const (
	statusNotStarted = "🔲 Not Started"
	statusInProgress = "🟡 In Progress"
	statusDone       = "🟢 Done"
)

var (
	roadmapHeading = regexp.MustCompile(`^## (\d+)\. (.+)$`)
	roadmapStatus  = regexp.MustCompile(`^\*\*Status:\*\*\s*(.+?)\s*$`)
	roadmapBox     = regexp.MustCompile(`^\s*- \[([ x])\]\s*(.*)$`)
)

// TestRoadmapStatusMatchesItsBoxes checks a roadmap item's stated status
// against the boxes underneath it.
//
// # Why this is a separate guard
//
// TestRoadmapBoxesMatchTheCode already checks a *box* against the code, and
// it was written because finished work left looking open sends the next
// contributor to build it twice. The status line one field above it was never
// checked against anything, and drifted the same way for the same reason:
// item 39 shipped in #74 with all five boxes ticked and "🔲 Not Started" left
// standing above them. Someone reading the roadmap for what to work on next
// would have found a finished feature advertised as untouched.
//
// A guard that catches one field and not its neighbour is how the second
// field becomes the one nobody trusts.
//
// # The rule
//
//   - Done: every required box is ticked, and there is at least one.
//   - Not Started: no box is ticked.
//   - In Progress: some but not all.
//
// A box whose text begins with "Optional" does not count toward Done. Item 32
// is Done with an optional MoonSep coupling deliberately not taken, and
// forcing that to In Progress would misreport a finished feature to satisfy
// arithmetic.
func TestRoadmapStatusMatchesItsBoxes(t *testing.T) {
	raw, err := os.ReadFile(roadmapDoc)
	if err != nil {
		t.Fatalf("read %s: %v", roadmapDoc, err)
	}

	type item struct {
		number, title, status  string
		line                   int
		ticked, required, opts int
	}

	var (
		items   []item
		current *item
	)

	flush := func() {
		if current != nil {
			items = append(items, *current)
			current = nil
		}
	}

	for n, line := range strings.Split(string(raw), "\n") {
		if m := roadmapHeading.FindStringSubmatch(line); m != nil {
			flush()

			current = &item{number: m[1], title: m[2], line: n + 1}

			continue
		}

		if current == nil {
			continue
		}

		if m := roadmapStatus.FindStringSubmatch(line); m != nil {
			current.status = m[1]

			continue
		}

		if m := roadmapBox.FindStringSubmatch(line); m != nil {
			switch {
			case strings.HasPrefix(strings.TrimSpace(m[2]), "Optional"):
				current.opts++
			default:
				current.required++

				if m[1] == "x" {
					current.ticked++
				}
			}
		}
	}

	flush()

	if len(items) == 0 {
		t.Fatalf("no numbered roadmap items found in %s", roadmapDoc)
	}

	var judged int

	for _, it := range items {
		if it.status == "" || it.required == 0 {
			continue
		}

		var want string

		switch it.ticked {
		case 0:
			want = statusNotStarted
		case it.required:
			want = statusDone
		default:
			want = statusInProgress
		}

		switch it.status {
		case statusNotStarted, statusInProgress, statusDone:
			judged++
		default:
			// A deliberate one-off label. Reported so a typo in one of the
			// three standard statuses cannot hide here unnoticed.
			t.Logf("item %s uses a non-standard status %q — not judged (%d/%d boxes ticked)",
				it.number, it.status, it.ticked, it.required)

			continue
		}

		if it.status != want {
			t.Errorf("%s:%d: item %s (%s) says %q but has %d of %d required boxes ticked, "+
				"which is %q.\n  Update whichever is wrong. A box beginning \"Optional\" is "+
				"excluded from this count; this item has %d.",
				roadmapDoc, it.line, it.number, it.title, it.status,
				it.ticked, it.required, want, it.opts)
		}
	}

	if judged == 0 {
		t.Fatal("no item used a standard status; the parser is probably wrong")
	}

	t.Logf("%d roadmap items, %d judged against their boxes", len(items), judged)
}
