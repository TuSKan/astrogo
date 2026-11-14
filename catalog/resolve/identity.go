package resolve

import "strings"

// simbadMarkers are the object-type prefixes SIMBAD writes before a name:
// "* alf CMa" for a star, "** STF 1234" for a double, "V* R Lep" for a
// variable, and "NAME LMC" for a common name. Another catalog writes the same
// object without them, OpenNGC's NGC 1990 listing "Eps Ori" where SIMBAD has
// "* eps Ori", so they are removed before two identifiers are compared.
var simbadMarkers = []string{"NAME ", "** ", "* ", "V* "}

// IdentityKeys returns the keys t is known by, for matching one catalog's
// entry to another's: its ID, designation and aliases, each with any SIMBAD
// object-type prefix removed and then put through [Normalize]. Empty
// identifiers give no key, and each key appears once.
//
// Two targets that share a key are the same object by identity. Nothing here
// looks at a position: two catalog entries a few arcseconds apart can be a
// star and its companion, or a star and the nebula around it, and only a
// shared identifier says they are one thing.
//
// The display Name is not a key. It is a label a provider chooses, and two
// different objects can carry the same one; a common name counts where a
// catalog lists it as an identifier, as SIMBAD's "NAME" identifiers and
// OpenNGC's common names are.
func IdentityKeys(t Target) []string {
	keys := make([]string, 0, 2+len(t.Aliases))
	seen := make(map[string]bool, cap(keys))

	add := func(id string) {
		id = strings.TrimSpace(id)
		for _, m := range simbadMarkers {
			if strings.HasPrefix(id, m) {
				id = id[len(m):]

				break
			}
		}

		if k := Normalize(id); k != "" && !seen[k] {
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
