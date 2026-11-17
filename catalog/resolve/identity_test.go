package resolve

import (
	"slices"
	"strings"
	"testing"
)

// sharesKey reports whether a and b have an identity key in common.
func sharesKey(a, b Target) bool {
	for _, k := range IdentityKeys(a) {
		if slices.Contains(IdentityKeys(b), k) {
			return true
		}
	}

	return false
}

func TestIdentityKeys(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		t    Target
		want int
	}{
		{"the display name is no key", Target{Name: "Alnilam"}, 0},
		{"blank identifiers give none", Target{ID: "M31", Designation: " ", Aliases: []string{""}}, 1},
		{"one designation in three spellings is one key", Target{ID: "NGC1990", Aliases: []string{"NGC 1990", "NGC  1990"}}, 1},
		{"ID, designation and each alias", Target{ID: "* eps Ori", Designation: "HD 37128", Aliases: []string{"HIP 26311", "NAME Alnilam"}}, 4},
	} {
		if got := IdentityKeys(c.t); len(got) != c.want {
			t.Errorf("%s: IdentityKeys = %q, want %d keys", c.name, got, c.want)
		}
	}
}

// TestIdentityKeysReadEachScheme: two spellings of one designation share a
// key, as their naming scheme says they are one, and designations the scheme
// tells apart do not. The spellings are the ones SIMBAD and OpenNGC write.
func TestIdentityKeysReadEachScheme(t *testing.T) {
	t.Parallel()

	same := [][]string{
		{"HD 037128", "HD  37128", "hd37128"},
		{"HIP 026311", "HIP 26311"},
		{"BD -01 0969", "BD-01   969"},
		{"TYC 4766-2450-1", "TYC 4766-2450-01"},
		{"NGC1990", "NGC  1990", "NGC 1990"},
		{"M31", "M  31", "Messier 31"},
		{"PGC 017223", "LEDA   17223"},
		{"ESO056-115", "ESO  56-115"},
		{"Eps Ori", "* eps Ori", "V* eps Ori", "epsilon ORI"},
		{"46 Ori", "*  46 Ori"},
		{"mu Cep", "* mu. Cep"},
		{"alf1 Cap", "* alf01 Cap", "Alpha1 CAP"},
		{"* b Vel", "*  b  Vel"},
		{"R Lep", "V* R Lep"},
		{"NAME Large Magellanic Cloud", "NAME large  magellanic cloud"},
		{"Gaia DR3 2202630001603369856", "gaia dr3 2202630001603369856"},
		{"IRAS 05240-6948", " IRAS 05240-6948 "},
	}

	for _, ids := range same {
		for _, other := range ids[1:] {
			if !sharesKey(Target{ID: ids[0]}, Target{ID: other}) {
				t.Errorf("%q and %q should share a key: %q, %q", ids[0], other,
					IdentityKeys(Target{ID: ids[0]}), IdentityKeys(Target{ID: other}))
			}
		}
	}

	apart := [][2]string{
		// A system and its components, and SIMBAD's capital component
		// suffix against a planet's lower-case one.
		{"* alf Cen", "* alf Cen A"},
		{"* alf Cen A", "* alf Cen B"},
		{"* alf Cen B", "alf Cen b"},
		{"CPD-60  5483", "CPD-60  5483A"},
		// Two stars a superscript tells apart.
		{"* alf01 Cru", "* alf02 Cru"},
		// One number in two Gaia releases, as SIMBAD lists R Lep.
		{"Gaia DR2 2987082722815713792", "Gaia DR3 2987082722815713792"},
		// A zone's sign is part of it.
		{"BD+00 123", "BD-00 123"},
		// A common name matches only a common name.
		{"NAME LMC", "LMC"},
		{"NAME Orion Nebula", "Orion Nebula"},
		// Two catalogs that share a prefix.
		{"HD 1234", "HDE 1234"},
		{"HR 1903", "HR Car"},
		// An identifier in no recognized scheme is compared as written.
		{"CEL    852", "CEL 852"},
		{"Cl* NGC 2516 ABC 1", "NGC 2516"},
	}

	for _, p := range apart {
		if sharesKey(Target{ID: p[0]}, Target{ID: p[1]}) {
			t.Errorf("%q and %q should not share a key: %q, %q", p[0], p[1],
				IdentityKeys(Target{ID: p[0]}), IdentityKeys(Target{ID: p[1]}))
		}
	}
}

// The identifiers SIMBAD lists for three objects (from its ids table, read
// through the CfA mirror on 2026-11-16), and OpenNGC's rows for two of them.
var (
	simbadEpsOri = strings.Split("TIC 427451176|1RXS J053619.2-011312|HIP 26311|2MASS J05361280-0112070|"+
		"PLX 1283|* eps Ori|*  46 Ori|2E  1411|AG-01  596|BD-01   969|CCDM J05363-0112A|CEL    852|"+
		"HD  37128|HIC  26311|HR  1903|IDS 05311-0116 A|NAME Alnilam|SAO 132346|V* eps Ori|"+
		"WDS J05362-0112A|TYC 4766-2450-1", "|")
	simbadNGC1990 = []string{"LBN   940", "LBN 205.42-17.78", "NGC  1990"}
	simbadLMC     = strings.Split("Anon 0524-69|ESO  56-115|ESO-LV  56-1150|GLXY G279.0-34.4+262|"+
		"IRAS 05240-6948|LEDA   17223|NAME LMC|NAME Large Magellanic Cloud|NAME Nubecula Major", "|")

	openngcNGC1990 = Target{ID: "NGC1990", Name: "Alnilam", Aliases: []string{"Alnilam", "BD -01 0969",
		"HD 037128", "HIP 026311", "IDS 05311-0116 A", "TYC 4766-2450-1", "WDS J05362-0112A", "Eps Ori", "46 Ori"}}
	openngcLMC = Target{ID: "ESO056-115", Name: "Large Magellanic Cloud", Aliases: []string{
		"Large Magellanic Cloud", "Nubecula Major", "ESO-LV 0561150", "IRAS 05240-6948", "PGC 017223"}}
)

// TestIdentityKeysMatchSIMBADToOpenNGC: OpenNGC's NGC 1990, which it
// catalogs as the star Alnilam, shares eight of SIMBAD's identifiers for
// eps Ori once each is read in its scheme, and its LMC three of SIMBAD's for
// the LMC (#1136). It also shares NGC 1990 with SIMBAD's nebula of that
// name, a separate object: a record can name two others, which is why a
// shared key alone does not decide a merge.
func TestIdentityKeysMatchSIMBADToOpenNGC(t *testing.T) {
	t.Parallel()

	shared := func(a, b Target) []string {
		var out []string

		for _, k := range IdentityKeys(a) {
			if slices.Contains(IdentityKeys(b), k) {
				out = append(out, k)
			}
		}

		return out
	}

	for _, c := range []struct {
		name   string
		simbad Target
		ngc    Target
		want   int
	}{
		{"Alnilam", Target{ID: "* eps Ori", Aliases: simbadEpsOri}, openngcNGC1990, 8},
		{"the LMC", Target{ID: "NAME LMC", Aliases: simbadLMC}, openngcLMC, 3},
		{"SIMBAD's nebula NGC 1990", Target{ID: "NGC  1990", Aliases: simbadNGC1990}, openngcNGC1990, 1},
	} {
		if got := shared(c.simbad, c.ngc); len(got) != c.want {
			t.Errorf("%s: SIMBAD and OpenNGC share %q, want %d keys", c.name, got, c.want)
		}
	}

	// The common names OpenNGC lists match none of SIMBAD's NAME
	// identifiers: they are in no namespace, so read as written.
	if sharesKey(Target{Aliases: []string{"NAME Alnilam"}}, Target{Aliases: []string{"Alnilam"}}) {
		t.Error("OpenNGC's common name Alnilam matched SIMBAD's NAME Alnilam")
	}
}
