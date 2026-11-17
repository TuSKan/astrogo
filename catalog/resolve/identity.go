package resolve

import (
	"regexp"
	"strings"

	"github.com/TuSKan/astrogo/constellation"
)

// IdentityKeys returns the keys t is known by, for matching one catalog's
// record to another's: one for its ID, its designation and each alias. Two
// targets that share a key are the same object by identity. Each key appears
// once, and an empty identifier gives none.
//
// An identifier is read in its naming scheme, and two forms give one key
// only where that scheme says they are one designation:
//   - SIMBAD's namespaces: "NAME Alnilam" is a common name, "* eps Ori" a
//     Bayer or Flamsteed designation, and "V* R Lep" a variable-star name.
//     A common name matches only another common name, ignoring case and
//     spacing.
//   - Bayer and Flamsteed designations ("eps Ori", "* mu. Cep", "46 Ori"),
//     with or without SIMBAD's "*": the Greek letter and the constellation
//     in any case or common abbreviation, and a superscript without its
//     padding, so "* alf01 Cap" and "Alpha1 CAP" are one.
//   - Variable-star names in the GCVS form ("R Lep", "V1500 Cyg").
//   - Numbered catalogs, where spacing and leading zeros carry no meaning:
//     "HD 037128" and SIMBAD's "HD  37128" are one, as are "M31" and
//     "Messier 31", and "PGC 017223" and "LEDA 17223", which are one
//     catalog. So are the structured numbers of ESO, BD/CD/CPD and TYC.
//     A Gaia source ID keeps its release, since "Gaia DR2 1" and "Gaia DR3 1"
//     need not be one source.
//
// A component suffix is kept as written in every scheme: "* alf Cen A" and
// "* alf Cen B" are two stars and "* alf Cen" is their system, and SIMBAD
// writes a star's components in capitals and its planets in lower case. An
// identifier in no recognized scheme is compared exactly as written.
//
// The keys are opaque and meant only for comparison. Nothing here looks at a
// position, and the display Name is never a key: it is a label a provider
// chooses, and two different objects can carry the same one.
func IdentityKeys(t Target) []string {
	keys := make([]string, 0, 2+len(t.Aliases))
	seen := make(map[string]bool, cap(keys))

	add := func(id string) {
		if k := identityKey(id); k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}

	add(t.ID)
	add(t.Designation)

	for _, a := range t.Aliases {
		add(a)
	}

	return keys
}

// identityKey returns the key one identifier is compared by, or "" for a
// blank one. A key starts with its scheme, so two schemes never collide.
func identityKey(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}

	switch {
	case strings.HasPrefix(id, "NAME "):
		return "name:" + strings.ToLower(strings.Join(strings.Fields(id[len("NAME "):]), " "))
	case strings.HasPrefix(id, "V* "):
		// The GCVS names a variable by its Bayer or Flamsteed designation
		// when it has one, so "V* eps Ori" is the star "* eps Ori". A Latin
		// letter here is a variable-star name, not a Bayer letter.
		if k, ok := starKey(id[len("V* "):], false); ok {
			return k
		}

		if k, ok := variableKey(id[len("V* "):]); ok {
			return k
		}
	case strings.HasPrefix(id, "* "):
		if k, ok := starKey(id[len("* "):], true); ok {
			return k
		}
	default:
		if k, ok := starKey(id, false); ok {
			return k
		}

		if k, ok := variableKey(id); ok {
			return k
		}

		if k, ok := catalogKey(id); ok {
			return k
		}
	}

	return "id:" + id
}

// greekLetters maps each spelling of a Greek letter a Bayer designation is
// written with to one form: SIMBAD's abbreviations ("mu." for the two-letter
// names), the other common abbreviations, and the full names.
var greekLetters = func() map[string]string {
	m := make(map[string]string)

	for canonical, spellings := range map[string][]string{
		"alf": {"alp", "alpha"}, "bet": {"beta"}, "gam": {"gamma"}, "del": {"delta"},
		"eps": {"epsilon"}, "zet": {"zeta"}, "eta": nil, "tet": {"the", "theta"},
		"iot": {"iota"}, "kap": {"kappa"}, "lam": {"lambda"}, "mu": nil, "nu": nil,
		"ksi": {"xi"}, "omi": {"omicron"}, "pi": nil, "rho": nil, "sig": {"sigma"},
		"tau": nil, "ups": {"upsilon"}, "phi": nil, "chi": nil, "psi": nil, "ome": {"omega"},
	} {
		m[canonical] = canonical
		for _, s := range spellings {
			m[s] = canonical
		}
	}

	return m
}()

// constellations maps each IAU constellation abbreviation, in upper case, to
// its standard form.
var constellations = func() map[string]string {
	m := make(map[string]string)
	for _, c := range constellation.List() {
		m[strings.ToUpper(c.Abbreviation)] = c.Abbreviation
	}

	return m
}()

// bayerLetter splits a Bayer designation's letter, "alf01", "Eps" or "mu.",
// into its letter and superscript.
var bayerLetter = regexp.MustCompile(`^([A-Za-z]+)\.?(\d*)$`)

// starKey reads s as a Bayer or Flamsteed designation: a Greek letter with an
// optional superscript, or a Flamsteed number, then a constellation and an
// optional component suffix. A Latin-letter Bayer designation ("b Vel") is
// read only where latin is set, under SIMBAD's "*", since without it a
// letter and a constellation are as likely a variable-star name.
func starKey(s string, latin bool) (string, bool) {
	f := strings.Fields(s)
	if len(f) < 2 || len(f) > 3 {
		return "", false
	}

	con, ok := constellations[strings.ToUpper(f[1])]
	if !ok {
		return "", false
	}

	var letter string

	switch m := bayerLetter.FindStringSubmatch(f[0]); {
	case m != nil && greekLetters[strings.ToLower(m[1])] != "":
		letter = greekLetters[strings.ToLower(m[1])] + unpadded(m[2])
	case isDigits(f[0]):
		letter = unpadded(f[0])
	case latin && len(f[0]) == 1 && isLetter(f[0][0]):
		letter = f[0]
	default:
		return "", false
	}

	key := "star:" + letter + " " + con
	if len(f) == 3 {
		key += " " + f[2]
	}

	return key, true
}

// gcvsName matches a variable-star name in the General Catalogue of Variable
// Stars' form: R to Z, a doubled letter (RR to ZZ, AA to QZ), or V and a
// number from 335 on.
var gcvsName = regexp.MustCompile(`^(?:[R-Z]|[R-Z][R-Z]|[A-Q][A-Z]|V\d{3,})$`)

// variableKey reads s as a GCVS variable-star name and its constellation.
func variableKey(s string) (string, bool) {
	f := strings.Fields(s)
	if len(f) != 2 || !gcvsName.MatchString(f[0]) {
		return "", false
	}

	con, ok := constellations[strings.ToUpper(f[1])]
	if !ok {
		return "", false
	}

	return "var:" + f[0] + " " + con, true
}

var (
	// numbered matches a designation in a catalog of plain numbers, where
	// spacing and leading zeros carry no meaning, with an optional suffix.
	numbered = regexp.MustCompile(`(?i)^(M|Messier|NGC|IC|HD|HIP|HR|SAO|PGC|LEDA|UGC|LBN)\s*0*(\d+)\s*([A-Za-z]*)$`)
	// eso matches an ESO (Uppsala) designation: field and number.
	eso = regexp.MustCompile(`(?i)^ESO\s*(\d+)\s*-\s*(\d+)\s*([A-Za-z]*)$`)
	// durchmusterung matches a BD, CD or CPD designation: zone and number.
	durchmusterung = regexp.MustCompile(`(?i)^(BD|CD|CPD)\s*([+-])\s*(\d+)\s+(\d+)\s*([A-Za-z]*)$`)
	// tycho matches a Tycho designation's three numbers.
	tycho = regexp.MustCompile(`(?i)^TYC\s*(\d+)\s*-\s*(\d+)\s*-\s*(\d+)$`)
	// gaia matches a Gaia source ID and the release it belongs to.
	gaia = regexp.MustCompile(`(?i)^Gaia\s+(E?DR\d+)\s+(\d+)$`)
)

// catalogKey reads s as a designation in one of the numbered catalogs. The
// leading acronym picks the catalog, so an identifier in none of them costs
// no pattern match.
func catalogKey(s string) (string, bool) {
	n := 0
	for n < len(s) && isLetter(s[n]) {
		n++
	}

	switch strings.ToUpper(s[:n]) {
	case "M", "MESSIER", "NGC", "IC", "HD", "HIP", "HR", "SAO", "PGC", "LEDA", "UGC", "LBN":
		m := numbered.FindStringSubmatch(s)
		if m == nil {
			return "", false
		}

		cat := strings.ToLower(m[1])

		switch cat {
		case "messier":
			cat = "m"
		case "leda":
			// LEDA numbers are PGC numbers: one catalog, two names.
			cat = "pgc"
		}

		return cat + ":" + unpadded(m[2]) + m[3], true
	case "ESO":
		if m := eso.FindStringSubmatch(s); m != nil {
			return "eso:" + unpadded(m[1]) + "-" + unpadded(m[2]) + m[3], true
		}
	case "BD", "CD", "CPD":
		if m := durchmusterung.FindStringSubmatch(s); m != nil {
			return strings.ToLower(m[1]) + ":" + m[2] + unpadded(m[3]) + " " + unpadded(m[4]) + m[5], true
		}
	case "TYC":
		if m := tycho.FindStringSubmatch(s); m != nil {
			return "tyc:" + unpadded(m[1]) + "-" + unpadded(m[2]) + "-" + unpadded(m[3]), true
		}
	case "GAIA":
		if m := gaia.FindStringSubmatch(s); m != nil {
			return "gaia:" + strings.ToUpper(m[1]) + " " + m[2], true
		}
	}

	return "", false
}

// unpadded returns a run of digits without its leading zeros: "0" for zero,
// and "" for no digits.
func unpadded(digits string) string {
	if digits == "" {
		return ""
	}

	if t := strings.TrimLeft(digits, "0"); t != "" {
		return t
	}

	return "0"
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}

	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}

	return true
}

func isLetter(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}
