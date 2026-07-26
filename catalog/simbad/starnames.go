package simbad

import "strings"

// starNames maps a SIMBAD Bayer/Flamsteed designation — as it appears in
// main_id with the leading "* " stripped and any trailing multiple-star
// component letter (" A", " B", ...) removed — to its IAU-approved common
// name (https://www.iau.org/public/themes/naming_stars/). Covers the
// ~150 brightest naked-eye stars likely to appear in a VisibleTonight-style
// query; SIMBAD's own main_id (e.g. "* alf CMa") is otherwise the only name
// astrogo would ever display for these, which is correct but not what most
// callers want to read as a display name.
//
// A few bright stars use a doubled Bayer-letter suffix for their multiple
// components (e.g. Acrux is "alf01 Cru"/"alf02 Cru", not "alf Cru" with an
// "A"/"B" suffix) — those are listed explicitly rather than relying on the
// generic component-letter stripping friendlyName does for the common case.
var starNames = map[string]string{
	// Andromeda
	"alf And": "Alpheratz",
	"bet And": "Mirach",
	"gam And": "Almach",
	// Aquila
	"alf Aql": "Altair",
	"bet Aql": "Alshain",
	"gam Aql": "Tarazed",
	// Aquarius
	"alf Aqr": "Sadalmelik",
	"bet Aqr": "Sadalsuud",
	// Aries
	"alf Ari": "Hamal",
	"bet Ari": "Sheratan",
	// Auriga
	"alf Aur": "Capella",
	"bet Aur": "Menkalinan",
	// Boötes
	"alf Boo": "Arcturus",
	"bet Boo": "Nekkar",
	"gam Boo": "Seginus",
	"eps Boo": "Izar",
	// Canis Major
	"alf CMa": "Sirius",
	"bet CMa": "Mirzam",
	"gam CMa": "Muliphein",
	"del CMa": "Wezen",
	"eps CMa": "Adhara",
	"zet CMa": "Furud",
	"eta CMa": "Aludra",
	// Canis Minor
	"alf CMi": "Procyon",
	"bet CMi": "Gomeisa",
	// Capricornus
	"alf Cap": "Algedi",
	"bet Cap": "Dabih",
	// Carina
	"alf Car": "Canopus",
	"bet Car": "Miaplacidus",
	"eps Car": "Avior",
	"iot Car": "Aspidiske",
	// Cassiopeia
	"alf Cas": "Schedar",
	"bet Cas": "Caph",
	"del Cas": "Ruchbah",
	"eps Cas": "Segin",
	// Centaurus
	"alf Cen": "Rigil Kentaurus",
	"bet Cen": "Hadar",
	// Cepheus
	"alf Cep": "Alderamin",
	"bet Cep": "Alfirk",
	"gam Cep": "Errai",
	// Cetus
	"alf Cet": "Menkar",
	"bet Cet": "Diphda",
	"omi Cet": "Mira",
	// Coma Berenices
	"alf Com": "Diadem",
	// Corona Borealis
	"alf CrB": "Alphecca",
	"bet CrB": "Nusakan",
	// Corvus
	"alf Crv": "Alchiba",
	"gam Crv": "Gienah",
	// Crux
	"alf01 Cru": "Acrux",
	"alf02 Cru": "Acrux",
	"bet Cru":   "Mimosa",
	"gam Cru":   "Gacrux",
	"del Cru":   "Imai",
	// Cygnus
	"alf Cyg": "Deneb",
	"bet Cyg": "Albireo",
	"gam Cyg": "Sadr",
	"eps Cyg": "Gienah",
	// Delphinus
	"alf Del": "Sualocin",
	"bet Del": "Rotanev",
	// Draco
	"alf Dra": "Thuban",
	"bet Dra": "Rastaban",
	"gam Dra": "Eltanin",
	// Equuleus
	"alf Equ": "Kitalpha",
	// Eridanus
	"alf Eri": "Achernar",
	"bet Eri": "Cursa",
	// Gemini
	"alf Gem": "Castor",
	"bet Gem": "Pollux",
	"gam Gem": "Alhena",
	// Grus
	"alf Gru": "Alnair",
	"bet Gru": "Tiaki",
	// Hercules
	"alf Her": "Rasalgethi",
	"bet Her": "Kornephoros",
	// Hydra
	"alf Hya": "Alphard",
	// Leo
	"alf Leo": "Regulus",
	"bet Leo": "Denebola",
	"gam Leo": "Algieba",
	"del Leo": "Zosma",
	"eps Leo": "Algenubi",
	// Lepus
	"alf Lep": "Arneb",
	"bet Lep": "Nihal",
	// Libra
	"alf Lib": "Zubenelgenubi",
	"bet Lib": "Zubeneschamali",
	// Lyra
	"alf Lyr": "Vega",
	"bet Lyr": "Sheliak",
	"gam Lyr": "Sulafat",
	// Ophiuchus
	"alf Oph": "Rasalhague",
	"bet Oph": "Cebalrai",
	// Orion
	"alf Ori": "Betelgeuse",
	"bet Ori": "Rigel",
	"gam Ori": "Bellatrix",
	"del Ori": "Mintaka",
	"eps Ori": "Alnilam",
	"zet Ori": "Alnitak",
	"kap Ori": "Saiph",
	// Pegasus
	"alf Peg": "Markab",
	"bet Peg": "Scheat",
	"gam Peg": "Algenib",
	"eps Peg": "Enif",
	"zet Peg": "Homam",
	// Perseus
	"alf Per": "Mirfak",
	"bet Per": "Algol",
	// Piscis Austrinus
	"alf PsA": "Fomalhaut",
	// Pisces
	"alf Psc": "Alrescha",
	// Puppis
	"zet Pup": "Naos",
	// Sagitta
	"alf Sge": "Sham",
	// Sagittarius
	"alf Sgr": "Rukbat",
	"eps Sgr": "Kaus Australis",
	"sig Sgr": "Nunki",
	"zet Sgr": "Ascella",
	"del Sgr": "Kaus Media",
	"lam Sgr": "Kaus Borealis",
	// Scorpius
	"alf Sco": "Antares",
	"bet Sco": "Acrab",
	"del Sco": "Dschubba",
	"lam Sco": "Shaula",
	"tet Sco": "Sargas",
	"kap Sco": "Girtab",
	"eps Sco": "Larawag",
	"sig Sco": "Alniyat",
	// Serpens
	"alf Ser": "Unukalhai",
	// Taurus
	"alf Tau": "Aldebaran",
	"bet Tau": "Elnath",
	"eta Tau": "Alcyone",
	// Triangulum Australe
	"alf TrA": "Atria",
	// Ursa Major
	"alf UMa":   "Dubhe",
	"bet UMa":   "Merak",
	"gam UMa":   "Phecda",
	"del UMa":   "Megrez",
	"eps UMa":   "Alioth",
	"zet UMa":   "Mizar",
	"zet01 UMa": "Mizar",
	"eta UMa":   "Alkaid",
	// Ursa Minor
	"alf UMi": "Polaris",
	"bet UMi": "Kochab",
	"gam UMi": "Pherkad",
	// Vela
	"gam Vel":   "Regor",
	"gam02 Vel": "Regor",
	// Virgo
	"alf Vir": "Spica",
	"bet Vir": "Zavijava",
	"gam Vir": "Porrima",
	"eps Vir": "Vindemiatrix",
	// Pavo
	"alf Pav": "Peacock",
	// Phoenix
	"alf Phe": "Ankaa",
}

// friendlyName returns mainID's IAU-approved common name when it's a
// recognized Bayer/Flamsteed designation, and true. Otherwise it returns
// mainID unchanged and false, so a caller can always use the first return
// value directly as a display name.
//
// SIMBAD's main_id for a named star looks like "* alf CMa" (the "* "
// prefix) or "* alf Cen A" (a trailing multiple-star component letter);
// both are normalized before lookup. Designations already using a doubled
// Bayer-letter suffix for their components (e.g. "alf01 Cru") are looked
// up as-is, matching starNames' explicit entries for those.
func friendlyName(mainID string) (name string, ok bool) {
	base := strings.TrimPrefix(mainID, "* ")

	if name, found := starNames[base]; found {
		return name, true
	}

	// Strip a trailing " A"/" B"/... component letter and retry — covers
	// the common case (e.g. "alf Cen A" -> "alf Cen") without needing a
	// separate entry per component.
	if idx := strings.LastIndexByte(base, ' '); idx > 0 && len(base)-idx == 2 {
		suffix := base[idx:]
		trimmed := base[:idx]

		if name, found := starNames[trimmed]; found {
			return name + suffix, true
		}
	}

	return mainID, false
}
