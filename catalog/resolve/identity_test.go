package resolve

import (
	"slices"
	"testing"
)

func TestIdentityKeys(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		t    Target
		want []string
	}{
		{
			"SIMBAD star: the marker is dropped, the display name is no key",
			Target{ID: "* eps Ori", Name: "Alnilam"},
			[]string{"epsori"},
		},
		{
			"OpenNGC: ID and identifiers, each once",
			Target{ID: "NGC1990", Name: "Alnilam", Aliases: []string{"NGC 1990", "Eps Ori", "HD 037128", "Alnilam"}},
			[]string{"ngc1990", "epsori", "hd037128", "alnilam"},
		},
		{
			"SIMBAD common name, double and variable markers",
			Target{ID: "NAME LMC", Designation: "** STF 1234", Aliases: []string{"V* R Lep"}},
			[]string{"lmc", "stf1234", "rlep"},
		},
		{
			"empty and blank identifiers give no key",
			Target{ID: "M31", Designation: " ", Aliases: []string{""}},
			[]string{"m31"},
		},
		{
			"a marker only at the start: an interior one is part of the name",
			Target{ID: "Cl* NGC 2516 ABC 1"},
			[]string{"cl*ngc2516abc1"},
		},
	} {
		if got := IdentityKeys(c.t); !slices.Equal(got, c.want) {
			t.Errorf("%s: IdentityKeys = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestIdentityKeysMatchAcrossCatalogs: the case #1136 reported, SIMBAD's
// Alnilam and OpenNGC's NGC 1990 (which OpenNGC catalogs as the star), share
// a key; a star and its companion, with identifiers of their own, do not;
// and two different objects that happen to carry the same display name do
// not either.
func TestIdentityKeysMatchAcrossCatalogs(t *testing.T) {
	t.Parallel()

	shares := func(a, b Target) bool {
		for _, k := range IdentityKeys(a) {
			if slices.Contains(IdentityKeys(b), k) {
				return true
			}
		}

		return false
	}

	for _, c := range []struct {
		name string
		a, b Target
		want bool
	}{
		{
			"SIMBAD's and OpenNGC's Alnilam",
			Target{ID: "* eps Ori", Name: "Alnilam"},
			Target{ID: "NGC1990", Name: "Alnilam", Aliases: []string{"Eps Ori", "HD 037128"}},
			true,
		},
		{
			"a system and its component",
			Target{ID: "* alf Cen", Name: "Rigil Kentaurus"},
			Target{ID: "* alf Cen A", Name: "Rigil Kentaurus A"},
			false,
		},
		{
			"two objects with one display name",
			Target{ID: "A1", Name: "TestObj"},
			Target{ID: "B2", Name: "TestObj"},
			false,
		},
	} {
		if got := shares(c.a, c.b); got != c.want {
			t.Errorf("%s: %q and %q share a key = %v, want %v", c.name, c.a.ID, c.b.ID, got, c.want)
		}
	}
}
