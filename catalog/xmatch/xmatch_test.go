package xmatch

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

func star(id, name string, aliases []string, ra, dec float64, hasCoord bool) resolve.Target {
	t := resolve.Target{ID: id, Name: name, Aliases: aliases}
	if hasCoord {
		t.Coord = coord.NewICRS(angle.Deg(ra), angle.Deg(dec))
		t.HasCoord = true
		t.Epoch = time.J2000()
	}

	return t
}

func TestMatch_AliasByID(t *testing.T) {
	a := []resolve.Target{star("HIP123", "Sirius", nil, 0, 0, false)}
	b := []resolve.Target{star("hip123", "Sirius (2)", nil, 0, 0, false)} // case/space-insensitive via resolve.Normalize

	pairs := Match(a, b)
	if len(pairs) != 1 {
		t.Fatalf("len(pairs) = %d, want 1", len(pairs))
	}

	if pairs[0].A.Name != "Sirius" || pairs[0].B.Name != "Sirius (2)" {
		t.Errorf("pairs[0] = %+v, want A=Sirius B=Sirius (2)", pairs[0])
	}
}

func TestMatch_AliasByAliasesEntry(t *testing.T) {
	a := []resolve.Target{star("SIMBAD-1", "Vega", []string{"HIP 91262", "HD 172167"}, 0, 0, false)}
	b := []resolve.Target{star("HIP 91262", "Vega (Gaia)", nil, 0, 0, false)}

	pairs := Match(a, b)
	if len(pairs) != 1 {
		t.Fatalf("len(pairs) = %d, want 1", len(pairs))
	}
}

func TestMatch_NoSharedAliasOrPosition_NoMatch(t *testing.T) {
	a := []resolve.Target{star("A1", "Alpha", nil, 10, 20, true)}
	b := []resolve.Target{star("B1", "Beta", nil, 200, -50, true)} // far away, no shared alias

	pairs := Match(a, b)
	if len(pairs) != 0 {
		t.Fatalf("len(pairs) = %d, want 0, got %+v", len(pairs), pairs)
	}
}

func TestMatch_PositionalFallbackWithinThreshold(t *testing.T) {
	a := []resolve.Target{star("A1", "Alpha", nil, 100.0, 20.0, true)}
	// 1 arcsec ~= 1/3600 deg away in RA.
	b := []resolve.Target{star("B1", "Alpha (other catalog)", nil, 100.0+1.0/3600.0*0.5, 20.0, true)}

	pairs := Match(a, b)
	if len(pairs) != 1 {
		t.Fatalf("len(pairs) = %d, want 1", len(pairs))
	}
}

func TestMatch_PositionalFallbackBeyondThreshold_NoMatch(t *testing.T) {
	a := []resolve.Target{star("A1", "Alpha", nil, 100.0, 20.0, true)}
	b := []resolve.Target{star("B1", "Alpha (far)", nil, 100.1, 20.0, true)} // ~0.1 deg away, way beyond 2 arcsec default

	pairs := Match(a, b)
	if len(pairs) != 0 {
		t.Fatalf("len(pairs) = %d, want 0, got %+v", len(pairs), pairs)
	}
}

func TestMatch_WithPositionMatchThreshold_WidensAcceptance(t *testing.T) {
	a := []resolve.Target{star("A1", "Alpha", nil, 100.0, 20.0, true)}
	b := []resolve.Target{star("B1", "Alpha (loose)", nil, 100.01, 20.0, true)} // ~36 arcsec away

	if pairs := Match(a, b); len(pairs) != 0 {
		t.Fatalf("default threshold: len(pairs) = %d, want 0", len(pairs))
	}

	pairs := Match(a, b, WithPositionMatchThreshold(angle.Arcsec(60)))
	if len(pairs) != 1 {
		t.Fatalf("widened threshold: len(pairs) = %d, want 1", len(pairs))
	}
}

func TestMatch_UntrustworthyCoordNeverMatchesPositionally(t *testing.T) {
	a := []resolve.Target{{ID: "A1", Name: "Alpha", HasCoord: true}} // HasCoord true but Coord is the zero value
	b := []resolve.Target{{ID: "B1", Name: "Alpha (dup)", HasCoord: true}}

	pairs := Match(a, b)
	if len(pairs) != 0 {
		t.Fatalf("len(pairs) = %d, want 0 (zero-coord targets must never positionally match)", len(pairs))
	}
}

func TestMatch_AliasGroupProducesOnePairPerCrossCombination(t *testing.T) {
	// Two entries in a share an alias with one entry in b (e.g. a
	// duplicate row within one catalog) — every (a, b) combination in the
	// resulting group should surface as its own Pair.
	a := []resolve.Target{
		star("DUP", "Alpha (row 1)", nil, 0, 0, false),
		star("DUP", "Alpha (row 2)", nil, 0, 0, false),
	}
	b := []resolve.Target{star("dup", "Alpha (catalog B)", nil, 0, 0, false)}

	pairs := Match(a, b)
	if len(pairs) != 2 {
		t.Fatalf("len(pairs) = %d, want 2, got %+v", len(pairs), pairs)
	}
}

func TestMatch_EmptyInputs(t *testing.T) {
	if pairs := Match(nil, nil); len(pairs) != 0 {
		t.Errorf("Match(nil, nil) = %+v, want empty", pairs)
	}

	a := []resolve.Target{star("A1", "Alpha", nil, 0, 0, false)}
	if pairs := Match(a, nil); len(pairs) != 0 {
		t.Errorf("Match(a, nil) = %+v, want empty", pairs)
	}
}

func TestMatch_EmptyIDAndAliasesNeverBucketed(t *testing.T) {
	// An item with no ID and only an empty-string alias must never create
	// a "" bucket that spuriously unions it with every other such item.
	a := []resolve.Target{{Name: "A", ID: "", Aliases: []string{""}}}
	b := []resolve.Target{{Name: "B", ID: "", Aliases: []string{""}}}

	pairs := Match(a, b)
	if len(pairs) != 0 {
		t.Fatalf("len(pairs) = %d, want 0 (empty ID/alias must never bucket-match)", len(pairs))
	}
}

func TestMatch_UnmatchedSingletonsProduceNoPair(t *testing.T) {
	a := []resolve.Target{
		star("A1", "Alpha", nil, 10, 20, true),
		star("A2", "Gamma", nil, 300, -10, true),
	}
	b := []resolve.Target{star("B1", "Alpha (match)", nil, 10, 20, true)}

	pairs := Match(a, b)
	if len(pairs) != 1 {
		t.Fatalf("len(pairs) = %d, want 1, got %+v", len(pairs), pairs)
	}

	if pairs[0].A.Name != "Alpha" {
		t.Errorf("matched pair A = %q, want Alpha", pairs[0].A.Name)
	}
}

// onDecLine places a Target arcsec east of RA 10°, Dec 20° along the parallel.
func onDecLine(id string, arcsec float64) resolve.Target {
	const dec = 20.0

	return star(id, id, nil, 10+arcsec/3600/math.Cos(dec*math.Pi/180), dec, true)
}

func pairIDs(pairs []Pair) []string {
	out := make([]string, len(pairs))
	for i, p := range pairs {
		out[i] = p.A.ID + "-" + p.B.ID
	}

	return out
}

// TestPositionalPassTakesTheClosestPairFirst (#728): a1, b1 and a2 lie 0″,
// 1.5″ and 1.6″ along one parallel, under the 2″ threshold. b1 is 0.1″ from
// a2 and 1.5″ from a1, so it pairs with a2 whatever order either slice is in.
// Until #728 the pass walked the items in order and each took its nearest
// partner not yet taken: [a1 a2] paired a1–b1, [a2 a1] paired a2–b1, and
// Match(b, a) paired b1–a2.
func TestPositionalPassTakesTheClosestPairFirst(t *testing.T) {
	t.Parallel()

	a1, a2, b1 := onDecLine("a1", 0), onDecLine("a2", 1.6), onDecLine("b1", 1.5)

	for _, c := range []struct {
		name string
		a, b []resolve.Target
		want string
	}{
		{"[a1 a2] against [b1]", []resolve.Target{a1, a2}, []resolve.Target{b1}, "a2-b1"},
		{"[a2 a1] against [b1]", []resolve.Target{a2, a1}, []resolve.Target{b1}, "a2-b1"},
		{"[b1] against [a1 a2]", []resolve.Target{b1}, []resolve.Target{a1, a2}, "b1-a2"},
	} {
		got := pairIDs(Match(c.a, c.b))
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("%s: %v, want [%s]", c.name, got, c.want)
		}
	}
}

// TestPositionalPassKeepsADuplicatedRow (#728): a row that appears twice in
// one slice is grouped with its copy by the alias pass, and that group still
// has no partner in the other slice. Until #728 only groups of one took part
// in the positional pass, so duplicating a row lost its match: one pair with
// the row once, none with it twice. Now the group pairs, and like any alias
// group it yields one Pair per cross-slice combination.
func TestPositionalPassKeepsADuplicatedRow(t *testing.T) {
	t.Parallel()

	m := star("HD 1", "HD 1", nil, 10, 20, true)
	g := star("Gaia 1", "Gaia 1", nil, 10, 20, true)

	if got := Match([]resolve.Target{m}, []resolve.Target{g}); len(got) != 1 {
		t.Errorf("row once: %d pairs, want 1", len(got))
	}

	if got := Match([]resolve.Target{m, m}, []resolve.Target{g}); len(got) != 2 {
		t.Errorf("row twice: %d pairs, want 2, one per copy", len(got))
	}
}
