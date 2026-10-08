package openngc

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// openNGCTypes is every object type OpenNGC's NGC_guide.txt defines, at the
// commit remote.OpenNGC pins, with whether it is an object to observe. The
// four that are not are a faded nova, an object that does not exist, a
// duplicate of another row, and "Other", which only its notes define.
var openNGCTypes = []struct {
	code       string
	observable bool
}{
	{"*", true}, {"**", true}, {"*Ass", true}, {"OCl", true}, {"GCl", true},
	{"Cl+N", true}, {"G", true}, {"GPair", true}, {"GTrpl", true}, {"GGroup", true},
	{"PN", true}, {"HII", true}, {"DrkN", true}, {"EmN", true}, {"Neb", true},
	{"RfN", true}, {"SNR", true},
	{"Nova", false}, {"NonEx", false}, {"Dup", false}, {"Other", false},
}

// Every observable OpenNGC type maps to a kind, and the rest are skipped.
// Before #599 the switch knew "*Assoc" and not "*Ass", and had no "EmN", so 72
// rows fell through to KindOther and were dropped: M24 and Brocchi's Cluster
// among them.
func TestMapKindKnowsEveryOpenNGCType(t *testing.T) {
	t.Parallel()

	for _, c := range openNGCTypes {
		got := mapKind(c.code)

		if c.observable && got == resolve.KindOther {
			t.Errorf("mapKind(%q) = KindOther; OpenNGC defines it as an object, and its rows are dropped", c.code)
		}

		if !c.observable && got != resolve.KindOther {
			t.Errorf("mapKind(%q) = %v; it is not an object to observe and should be skipped", c.code, got)
		}
	}
}

// Rows copied verbatim from the pinned NGC.csv and addendum.csv, with their
// real headers: what the parser meets in production, not a fixture written to
// suit it. M24 and Brocchi's Cluster are both "*Ass", an association of stars,
// and NGC 1936 is "EmN", an emission nebula.
const (
	realNGCRows = `Name;Type;RA;Dec;Const;MajAx;MinAx;PosAng;B-Mag;V-Mag;J-Mag;H-Mag;K-Mag;SurfBr;Hubble;Pax;Pm-RA;Pm-Dec;RadVel;Redshift;Cstar U-Mag;Cstar B-Mag;Cstar V-Mag;M;NGC;IC;Cstar Names;Identifiers;Common names;NED notes;OpenNGC notes;Sources
IC4715;*Ass;18:16:56.12;-18:30:52.4;Sgr;120.00;60.00;90;;4.50;;;;;;;;;;;;;;024;;;;;Small Sgr Star Cloud;Milky Way star cloud.;;Type:1|RA:1|Dec:1|Const:99|MajAx:2|MinAx:2|PosAng:2|V-Mag:10
NGC1936;EmN;05:22:13.96;-67:58:41.9;Dor;1.00;1.00;;;11.60;16.03;14.51;12.87;;;;;;;;;;;;;2127;;IRAS 05223-6801;;Part of a large star-formation complex in the LMC.;;Type:1|RA:1|Dec:1|Const:99|MajAx:7|MinAx:7|V-Mag:2|J-Mag:2|H-Mag:2|K-Mag:2
NGC1976;Cl+N;05:35:16.48;-05:23:22.8;Ori;90.00;60.00;;4.00;4.00;;;;;;;1.670;-0.300;28;0.000093;;;;042;;;;LBN 974,MWSC 0582;Great Orion Nebula,Orion Nebula;;;Type:1|RA:1|Dec:1|Const:99|MajAx:9|MinAx:9|B-Mag:3|V-Mag:10|Pm-RA:2|Pm-Dec:2|RadVel:2|Redshift:2
`
	realAddendumRows = `Name;Type;RA;Dec;Const;MajAx;MinAx;PosAng;B-Mag;V-Mag;J-Mag;H-Mag;K-Mag;SurfBr;Hubble;Pax;Pm-RA;Pm-Dec;RadVel;Redshift;Cstar U-Mag;Cstar B-Mag;Cstar V-Mag;M;NGC;IC;Cstar Names;Identifiers;Common names;NED notes;OpenNGC notes;Sources
Cl399;*Ass;19:25:24.0;+20:11:00;Vul;70.00;;;3.93;3.60;;;;;;;;;;;;;;;;;;;Brocchi's Cluster,Al Sufi's Cluster,Coathanger Asterism;;;Type:2|RA:2|Dec:2|Const:99|MajAx:99|B-Mag:2|V-Mag:2
`
)

func TestParseKeepsAssociationsAndEmissionNebulae(t *testing.T) {
	t.Parallel()

	byID := map[string]targetRecord{}

	for _, src := range []string{realNGCRows, realAddendumRows} {
		recs, err := parseOpenNGC(strings.NewReader(src))
		if err != nil {
			t.Fatalf("parseOpenNGC: %v", err)
		}

		for _, r := range recs {
			byID[r.ID] = r
		}
	}

	cases := []struct {
		id, name, vmag string
		kind           resolve.Kind
		alias          string
	}{
		{"IC4715", "Small Sgr Star Cloud", "4.50", resolve.KindStarCluster, "M 24"},
		{"NGC1936", "NGC1936", "11.60", resolve.KindNebula, ""},
		{"NGC1976", "Great Orion Nebula", "4.00", resolve.KindOpenCluster, "M 42"},
		{"CL399", "Brocchi's Cluster", "3.60", resolve.KindStarCluster, "Coathanger Asterism"},
	}

	for _, c := range cases {
		r, ok := byID[c.id]
		if !ok {
			t.Errorf("%s was not parsed; the rows are OpenNGC's own", c.id)
			continue
		}

		if r.Name != c.name || r.VMag != c.vmag || r.Kind != c.kind {
			t.Errorf("%s parsed as name %q, V %q, kind %v; want %q, %q, %v",
				c.id, r.Name, r.VMag, r.Kind, c.name, c.vmag, c.kind)
		}

		if c.alias == "" {
			continue
		}

		found := false

		for _, a := range r.Aliases {
			if a == c.alias {
				found = true
			}
		}

		if !found {
			t.Errorf("%s has aliases %v, want %q among them", c.id, r.Aliases, c.alias)
		}
	}
}
