package xmatch

import (
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
		t.Epoch = time.J2000
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
