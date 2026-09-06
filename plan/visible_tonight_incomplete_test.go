package plan_test

import (
	"context"
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
)

var errSourceUnreachable = errors.New("visible_tonight_incomplete_test: catalog source unreachable")

// failingBrightSource yields one good target and then fails, the shape of a
// provider that goes away partway through a bulk listing — a paged query whose
// second page times out, a connection dropped mid-stream.
//
// It yields the good row FIRST deliberately: a source that fails immediately
// would be indistinguishable from one with nothing to say, and the interesting
// case is precisely the one where a caller gets real results back and has no
// way to know they are short.
type failingBrightSource struct {
	good resolve.Target
}

func (f *failingBrightSource) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapMagnitudeBrowse}
}

func (f *failingBrightSource) SearchBright(_ context.Context, _ resolve.BrightRequest) resolve.SeqIterator[resolve.Target] {
	return func(yield func(resolve.Target, error) bool) {
		if !yield(f.good, nil) {
			return
		}

		yield(resolve.Target{}, errSourceUnreachable)
	}
}

// TestVisibleTonightReportsAnIncompleteResult is the caller-facing half of the
// #177 family, and the reason the skip logging alone was not enough.
//
// VisibleTonight is documented to skip what it cannot evaluate rather than fail
// the night — that behaviour is right and does not change here. What changed is
// that the skip stopped being invisible: it used to leave only a Warn line,
// which a program cannot branch on, so a night's list came back short with the
// same nil error as a night that was genuinely quiet.
//
// The test asserts both halves at once, because either alone is a different
// bug: Sirius must still be in the results (skipping stayed a skip, not a
// failure), AND the error must wrap ErrIncomplete (the shortfall is reported).
func TestVisibleTonightReportsAnIncompleteResult(t *testing.T) {
	sources := []resolve.BrightObjectSearcher{&failingBrightSource{good: sirius}}

	results, err := plan.VisibleTonight(context.Background(), quintaCalixtoSite(t), testNight, 2, sources, ephemeris.Default())

	if !errors.Is(err, plan.ErrIncomplete) {
		t.Errorf("VisibleTonight returned err = %v, want it to wrap ErrIncomplete.\n"+
			"  A source failed mid-listing, so this night's sky is short by an unknown number of objects "+
			"and the caller has no way to tell that from a quiet sky.", err)
	}

	if !errors.Is(err, errSourceUnreachable) {
		t.Errorf("the underlying cause was lost: %v", err)
	}

	if _, ok := findByName(results, "Sirius"); !ok {
		t.Errorf("Sirius is missing from %+v.\n"+
			"  The failure must degrade the result, not replace it — one bad source "+
			"costs the caller that source, not the night.", results)
	}
}

// TestVisibleTonightReturnsNilErrorWhenComplete is the other half, and the one
// that gives the signal its meaning.
//
// If a clean run also returned ErrIncomplete, `errors.Is(err, ErrIncomplete)`
// would be true always and would tell a caller nothing. This is the assertion
// that would catch a future gatherer recording a skip for an ordinary
// non-result — a target that is simply below the magnitude limit, say.
func TestVisibleTonightReturnsNilErrorWhenComplete(t *testing.T) {
	faint := sirius
	faint.ID, faint.Name, faint.VMag = "faint", "Faint Star", 6.0

	sources := []resolve.BrightObjectSearcher{&mockBrightSource{targets: []resolve.Target{sirius, faint}}}

	results, err := plan.VisibleTonight(context.Background(), quintaCalixtoSite(t), testNight, 2, sources, ephemeris.Default())
	if err != nil {
		t.Fatalf("a run in which nothing failed returned %v, want nil.\n"+
			"  A signal that is always on is not a signal.", err)
	}

	// Precondition: this run really did reject a candidate, so the nil error
	// above means "rejections are not skips" rather than "nothing happened".
	if _, ok := findByName(results, "Faint Star"); ok {
		t.Fatal("precondition: Faint Star (VMag 6) should have been filtered out at magLimit 2")
	}

	if _, ok := findByName(results, "Sirius"); !ok {
		t.Fatalf("precondition: expected Sirius in %+v", results)
	}
}
