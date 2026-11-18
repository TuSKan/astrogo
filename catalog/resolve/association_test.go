package resolve_test

import (
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// TestParseIdentifierReadsTheScheme: spellings of one designation read as one
// Identifier, a Gaia source ID keeps its release, a name in no scheme is its
// own text, and a blank string is the zero Identifier.
func TestParseIdentifierReadsTheScheme(t *testing.T) {
	t.Parallel()

	if a, b := resolve.ParseIdentifier("HD 037128"), resolve.ParseIdentifier("HD  37128"); a != b || a.Namespace != "hd" || a.Value != "37128" {
		t.Errorf("HD 037128 = %+v, HD  37128 = %+v; want one Identifier, hd 37128", a, b)
	}

	if got, want := resolve.ParseIdentifier("Gaia DR3 4472832130942575872"), (resolve.Identifier{Namespace: "gaia", Release: "DR3", Value: "4472832130942575872"}); got != want {
		t.Errorf("Gaia DR3 = %+v, want %+v", got, want)
	}

	if a, b := resolve.ParseIdentifier("Gaia DR2 1"), resolve.ParseIdentifier("Gaia DR3 1"); a == b {
		t.Errorf("Gaia DR2 1 and DR3 1 read as one Identifier, %+v", a)
	}

	if got := resolve.ParseIdentifier("row-17"); got.Namespace != "id" || got.Value != "row-17" {
		t.Errorf("row-17 = %+v, want the id namespace holding it as written", got)
	}

	if got := resolve.ParseIdentifier("  "); got != (resolve.Identifier{}) {
		t.Errorf("blank = %+v, want the zero Identifier", got)
	}
}

// TestIdentifiersFollowIdentityKeys: Identifiers gives one Identifier for
// each identity key, in order.
func TestIdentifiersFollowIdentityKeys(t *testing.T) {
	t.Parallel()

	target := resolve.Target{ID: "* alf CMa", Designation: "HIP 32349", Aliases: []string{"HD 48915", "NAME Sirius"}}

	keys, ids := resolve.IdentityKeys(target), resolve.Identifiers(target)
	if len(keys) != len(ids) || len(ids) != 4 {
		t.Fatalf("%d keys, %d identifiers; want four of each", len(keys), len(ids))
	}

	if ids[1] != (resolve.Identifier{Namespace: "hip", Value: "32349"}) || ids[3].Namespace != "name" {
		t.Errorf("Identifiers = %+v", ids)
	}
}
