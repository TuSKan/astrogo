package resolve_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// An exact match must score one whether or not the caller normalized first.
//
// Only the candidate used to be normalized, which made a pre-normalized query
// an unstated precondition — catalog's resolver met it, catalog/simbad's did
// not. An exact match of "NGC 224" against "NGC 224" scored 0.129: the
// capitals and the space pushed it past the equality test, past the prefix and
// substring tests, and into the Levenshtein term, whose ceiling is 0.3.
func TestScoreDoesNotRequireAPreNormalizedQuery(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		query, candidate string
	}{
		{"Betelgeuse", "Betelgeuse"},
		{"betelgeuse", "Betelgeuse"},
		{"BETELGEUSE", "betelgeuse"},
		{"M 31", "M 31"},
		{"M31", "M 31"},
		{"M 31", "M31"},
		{"NGC 224", "NGC 224"},
		{"ngc224", "NGC 224"},
		{"  NGC 224  ", "NGC224"},
		{"Messier 31", "M31"},
		{"messier31", "M 31"},
	} {
		if got := resolve.Score(c.query, c.candidate); math.Abs(got-1) > 1e-12 {
			t.Errorf("Score(%q, %q) = %.4f, want 1 — these name the same object", c.query, c.candidate, got)
		}
	}

	// Passing an already-normalized query gives the same answer, since
	// Normalize is idempotent. That is what makes normalizing inside Score
	// free for the caller that already did it.
	for _, q := range []string{"M 31", "NGC 224", "Messier 31", "Betelgeuse"} {
		raw := resolve.Score(q, "M31")
		pre := resolve.Score(resolve.Normalize(q), "M31")

		if raw != pre {
			t.Errorf("Score(%q, ...) = %.4f but Score(Normalize(%q), ...) = %.4f", q, raw, q, pre)
		}
	}
}

// The rungs of the scale must stay ordered: exact beats prefix beats substring
// beats an edit-distance guess. That ordering is the whole point of the
// function, and it is what collapsed when everything fell into the last rung.
func TestScoreRungsAreOrdered(t *testing.T) {
	t.Parallel()

	const query = "NGC 224"

	exact := resolve.Score(query, "NGC 224")
	prefix := resolve.Score(query, "NGC 2244")
	substring := resolve.Score(query, "XNGC224Y")
	distant := resolve.Score(query, "NGC 7331")

	if !(exact > prefix && prefix > substring && substring > distant) {
		t.Errorf("the scale is not ordered: exact %.4f, prefix %.4f, substring %.4f, distant %.4f",
			exact, prefix, substring, distant)
	}

	if exact != 1 {
		t.Errorf("an exact match scores %.4f, want 1", exact)
	}

	// And every score stays inside the range the doc claims.
	for _, candidate := range []string{
		"NGC 224", "NGC 2244", "XNGC224Y", "NGC 7331", "Andromeda", "", "M31",
		"a", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	} {
		if s := resolve.Score(query, candidate); s < 0 || s > 1 {
			t.Errorf("Score(%q, %q) = %v, outside [0, 1]", query, candidate, s)
		}
	}

	// An empty side matches nothing rather than everything.
	if s := resolve.Score("", "NGC 224"); s != 0 {
		t.Errorf("an empty query scored %v against a real name, want 0", s)
	}

	if s := resolve.Score(query, ""); s != 0 {
		t.Errorf("a real query scored %v against an empty name, want 0", s)
	}

	// A query that normalizes away to nothing is empty too, and must not
	// suddenly match by way of the substring rung.
	if s := resolve.Score("   ", "NGC 224"); s > 0.5 {
		t.Errorf("a query of only spaces scored %v", s)
	}
}

// Normalize must be idempotent, since Score now applies it to input that may
// already have been through it.
func TestNormalizeIsIdempotent(t *testing.T) {
	t.Parallel()

	for _, q := range []string{
		"", "   ", "M31", "M 31", "Messier 31", "MESSIER 31", "messier31",
		"NGC 224", "  Betelgeuse  ", "Alpha Centauri A", "messier", "Messier",
	} {
		once := resolve.Normalize(q)
		if twice := resolve.Normalize(once); twice != once {
			t.Errorf("Normalize(%q) = %q, but normalizing that gives %q", q, once, twice)
		}
	}

	// The Messier expansion is what it claims to be, and does not run off the
	// end of a string that is exactly the prefix.
	for _, c := range []struct{ in, want string }{
		{"Messier 31", "m31"},
		{"MESSIER31", "m31"},
		{"messier 1", "m1"},
		{"messier", "m"},
		{"M 31", "m31"},
	} {
		if got := resolve.Normalize(c.in); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
