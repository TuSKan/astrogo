package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
)

// americanSpelling maps each British spelling this repository has used to the
// American one it uses instead.
var americanSpelling = map[string]string{
	"acknowledgement": "acknowledgment", "acknowledgements": "acknowledgments",
	"amortised": "amortized", "amortises": "amortizes",
	"analogue": "analog", "analogues": "analogs",
	"apologising": "apologizing",
	"artefact":    "artifact", "artefacts": "artifacts",
	"authorise": "authorize", "authorised": "authorized",
	"behaviour": "behavior", "behaviours": "behaviors", "behavioural": "behavioral",
	"cancelled": "canceled", "cancelling": "canceling",
	"catalogue": "catalog", "catalogues": "catalogs", "catalogued": "cataloged", "cataloguing": "cataloging",
	"categorise": "categorize", "categorised": "categorized",
	"centimetre": "centimeter", "centimetres": "centimeters",
	"centralise": "centralize", "centralising": "centralizing",
	"centre": "center", "centres": "centers", "centred": "centered", "centring": "centering",
	"characterisation": "characterization", "characterise": "characterize",
	"characterised": "characterized", "characterises": "characterizes",
	"colour": "color", "colours": "colors", "coloured": "colored", "colouring": "coloring",
	"customise": "customize", "customised": "customized",
	"defence": "defense", "offence": "offense",
	"depolarisation": "depolarization", "depolarising": "depolarizing",
	"deprioritised":  "deprioritized",
	"digitised":      "digitized",
	"discretisation": "discretization", "discretises": "discretizes",
	"emphasise": "emphasize", "emphasised": "emphasized", "emphasises": "emphasizes",
	"epicentre":     "epicenter",
	"factorisation": "factorization",
	"favour":        "favor", "favours": "favors", "favoured": "favored", "favouring": "favoring", "favourable": "favorable",
	"finalise": "finalize", "finalised": "finalized",
	"generalised": "generalized", "generalises": "generalizes", "generalising": "generalizing",
	"grey": "gray", "greys": "grays", "greyed": "grayed",
	"honour": "honor", "honours": "honors", "honoured": "honored", "honouring": "honoring",
	"idealised":      "idealized",
	"initialisation": "initialization", "initialise": "initialize", "initialised": "initialized",
	"initialises": "initializes", "uninitialised": "uninitialized",
	"judgement": "judgment", "judgements": "judgments",
	"kilometre": "kilometer", "kilometres": "kilometers",
	"labelled": "labeled", "labelling": "labeling",
	"licence": "license", "licences": "licenses",
	"localisation": "localization", "localised": "localized", "localises": "localizes",
	"materialised": "materialized", "materialises": "materializes", "materialising": "materializing",
	"maximisation": "maximization", "maximises": "maximizes",
	"memorise": "memorize", "memorised": "memorized",
	"metre": "meter", "metres": "meters", "millimetre": "millimeter", "millimetres": "millimeters",
	"minimised": "minimized", "minimises": "minimizes",
	"modelled": "modeled", "modelling": "modeling", "modeller": "modeler",
	"neighbour": "neighbor", "neighbours": "neighbors", "neighbouring": "neighboring", "neighbourhood": "neighborhood",
	"normalise": "normalize", "normalised": "normalized", "normalises": "normalizes",
	"optimisation": "optimization", "optimise": "optimize", "optimised": "optimized", "optimiser": "optimizer",
	"organise": "organize", "organised": "organized", "organises": "organizes",
	"parallelises":     "parallelizes",
	"parameterisation": "parameterization", "parameterise": "parameterize", "parameterised": "parameterized",
	"parameterises": "parameterizes", "parameterising": "parameterizing",
	"penalised":  "penalized",
	"prioritise": "prioritize", "prioritised": "prioritized",
	"programme": "program", "programmes": "programs",
	"quantisation": "quantization", "quantised": "quantized", "quantises": "quantizes",
	"randomised": "randomized", "randomises": "randomizes",
	"realise": "realize", "realised": "realized", "realises": "realizes",
	"recognise": "recognize", "recognised": "recognized", "recognises": "recognizes",
	"sanitised":     "sanitized",
	"serialisation": "serialization", "serialisations": "serializations", "serialise": "serialize",
	"serialised": "serialized", "serialises": "serializes",
	"signalled":   "signaled",
	"specialised": "specialized",
	"spelt":       "spelled",
	"stabilises":  "stabilizes",
	"standardise": "standardize", "standardised": "standardized",
	"summarise": "summarize", "summarised": "summarized", "summariser": "summarizer",
	"summarises": "summarizes", "summarising": "summarizing",
	"synchronisation": "synchronization", "synchronised": "synchronized",
	"synthesised": "synthesized", "synthesises": "synthesizes", "synthesising": "synthesizing",
	"theatre":    "theater",
	"tokenising": "tokenizing",
	"totalled":   "totaled",
	"towards":    "toward",
	"travelled":  "traveled", "travelling": "traveling", "traveller": "traveler",
	"utilise": "utilize", "utilised": "utilized", "utilises": "utilizes",
	"vapour": "vapor", "vapours": "vapors",
	"visualise": "visualize", "visualised": "visualized",
}

// britishAllowed are the spans that keep a British spelling because it is
// someone else's: a proper name, a published title, or an identifier an
// external service defines. Each is masked before the check.
var britishAllowed = []*regexp.Regexp{
	// Gaia archive table names.
	regexp.MustCompile(`hipparcos2_best_neighbour|tycho2tdsc_merge_best_neighbour`),
	// The Canadian Space Weather Forecast Centre's name, also across a
	// wrapped comment line.
	regexp.MustCompile(`Forecast(?:\s*(?://\s*)?)Centre`),
	// A catalog's published title: the Bright Star Catalogue, the Hipparcos
	// Catalogue, "Catalogue of …".
	regexp.MustCompile(`\b(?:The |A |This |That |Each |Every |One )?[A-Z][A-Za-z0-9\-]* Catalogues?\b|\bCatalogues? of [A-Z]`),
	// Paper titles: Falchi et al.'s, and Levine, Tavella & Milton (2023).
	regexp.MustCompile(`Towards a global map of the artificial all-sky brightness|Towards a consensus on a`),
	// Gaia's own VOTable field description, reproduced in a fixture.
	regexp.MustCompile(`BP - RP colour`),
}

// inlineCode matches a Markdown inline code span.
var inlineCode = regexp.MustCompile("`[^`\n]+`")

// TestAmericanEnglish keeps the repository's own prose and identifiers in
// American English: meters, not metres; center, not centre.
//
// # Why
//
// unit/ settled on "meter" long before the rest of the tree was held to it,
// and a sweep found 707 British spellings in 275 files, "centre" among them
// in identifiers a caller types. A convention nobody checks drifts back one
// comment at a time, so this checks it.
//
// # Scope
//
// Every *.go, *.md, *.yml and *.yaml file, examples/ included. Not
// covered:
//
//   - CHANGELOG.md, whose released entries are never rewritten;
//   - testdata/, which holds other people's bytes as they sent them;
//   - this file, which has to name the spellings it rejects.
//
// Someone else's name or title keeps its spelling: those spans are listed in
// britishAllowed, each with its reason.
func TestAmericanEnglish(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var checked int

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}

			return nil
		}

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil //nolint:nilerr // an unrelatable path is skipped, not fatal
		}

		rel = filepath.ToSlash(rel)

		switch {
		case rel == "CHANGELOG.md", rel == "internal/docsguard/americanenglish_test.go":
			return nil
		case !strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, ".md") &&
			!strings.HasSuffix(rel, ".yml") && !strings.HasSuffix(rel, ".yaml"):
			return nil
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // an unreadable file is skipped, not fatal
		}

		checked++

		text := string(data)
		for _, allowed := range britishAllowed {
			text = allowed.ReplaceAllStringFunc(text, func(s string) string { return strings.Repeat("_", len(s)) })
		}

		// In Markdown an inline code span quotes code as it is or was, a
		// renamed identifier among them; the prose around it is what is
		// checked here, and live identifiers are checked in the .go files.
		if strings.HasSuffix(rel, ".md") {
			text = inlineCode.ReplaceAllStringFunc(text, func(s string) string { return strings.Repeat("_", len(s)) })
		}

		for i, line := range strings.Split(text, "\n") {
			for _, word := range words(line) {
				if american, ok := americanSpelling[strings.ToLower(word)]; ok {
					t.Errorf("%s:%d: %q is British; write %q", rel, i+1, word, american)
				}
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if checked < 500 {
		t.Fatalf("checked only %d files; the walk is not seeing the repository", checked)
	}
}

// words splits a line into its words, and an identifier into the words it
// is made of: worldAtTheCentre gives world, At, The and Centre, and
// HTTPServer gives HTTP and Server.
func words(line string) []string {
	var out []string

	r := []rune(line)
	start := -1

	for i := 0; i <= len(r); i++ {
		letter := i < len(r) && unicode.IsLetter(r[i])

		boundary := !letter
		if letter && start >= 0 && unicode.IsUpper(r[i]) {
			prev := r[i-1]
			nextLower := i+1 < len(r) && unicode.IsLower(r[i+1])
			boundary = unicode.IsLower(prev) || (unicode.IsUpper(prev) && nextLower)
		}

		if boundary && start >= 0 {
			out = append(out, string(r[start:i]))
			start = -1
		}

		if letter && start < 0 {
			start = i
		}
	}

	return out
}
