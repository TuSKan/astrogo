//go:build validation

package jpl_test

import (
	"math"
	"testing"
)

// TestCorpusQueryIsUnchangedSinceItWasGenerated answers the cheapest of the
// three explanations for a corpus diff: that astrogo changed what it asks.
//
// # Why this needs a test rather than a reading
//
// When TestGenerateCorpus reports entries "changed", the natural conclusion is
// that Horizons changed its answer. But an entry is keyed on class, body, site
// *name* and epoch — not on the site's coordinates — so moving a site by a
// metre, or getting its height wrong by a factor, reports as 300 changed values
// rather than as a changed query. The two look identical in the summary and
// mean opposite things.
//
// That is not a hypothetical shape of mistake. The site heights passed to
// Horizons went through the typed-quantities migration in #130, where a
// `loc.Height()` became a `loc.Height().Meters()`; a wrong unit there is
// exactly the silent, plausible, whole-corpus shift this catches. It would move
// the real sites and leave the two synthetic ones at height zero untouched,
// which is a pattern worth being able to recognize.
//
// So the manifest's record of what was asked is compared against what the code
// would ask today. If they differ, the diff is astrogo's and no amount of
// -update-corpus is the right response.
//
// Offline: this reads the checked-in corpus and the site table, and asks
// nothing of anybody.
func TestCorpusQueryIsUnchangedSinceItWasGenerated(t *testing.T) {
	t.Parallel()

	c, err := loadCorpus()
	if err != nil {
		t.Fatalf("loadCorpus: %v", err)
	}

	today := corpusSites(t)

	byName := make(map[string]corpusSite, len(today))
	for _, s := range today {
		byName[s.Name] = s
	}

	if len(today) != len(c.Manifest.Sites) {
		t.Errorf("the corpus was generated from %d sites and the code offers %d today; "+
			"a site added or dropped changes which entries exist, not just their values",
			len(c.Manifest.Sites), len(today))
	}

	// Exact equality. These numbers are carried into the query as decimal
	// strings, so "close enough" is not a property the reference has: a
	// different number is a different question.
	for _, was := range c.Manifest.Sites {
		now, ok := byName[was.Name]
		if !ok {
			t.Errorf("the corpus holds entries for site %q, which the code no longer defines; "+
				"its entries are answers to a question nobody asks any more", was.Name)

			continue
		}

		for _, f := range []struct {
			what     string
			was, now float64
		}{
			{"longitude", was.Lon, now.Lon},
			{"latitude", was.Lat, now.Lat},
			{"height", was.Height, now.Height},
		} {
			if f.was != f.now {
				t.Errorf("%s %s: the corpus was generated asking %.17g, the code asks %.17g today "+
					"(by %.3g).\n"+
					"  Every entry for this site is an answer to the older question, so a\n"+
					"  corpus diff here is astrogo's change and not Horizons'. Fix the site\n"+
					"  or regenerate deliberately — do not read the diff as upstream drift.",
					was.Name, f.what, f.was, f.now, math.Abs(f.was-f.now))
			}
		}
	}

	t.Logf("%d sites, unchanged since %s (astrogo %.12s)",
		len(c.Manifest.Sites), c.Manifest.Generated, c.Manifest.Commit)
}
