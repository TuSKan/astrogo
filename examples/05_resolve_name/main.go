// Package main demonstrates astronomical name resolution.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/examples/internal/show"
	"github.com/TuSKan/astrogo/remote"
	_ "github.com/TuSKan/astrogo/remote/eop"
)

func main() {
	// OpenNGC fetches its catalog data over the network, like every other
	// astrogo catalog provider (see README "Data downloads & offline
	// usage"). Enabling downloads here is enough — catalog.NewResolver's
	// first use of catalog.OpenNGC below fetches it automatically
	// (content-checked, so a re-run only costs a HEAD probe once cached).
	// No need to import catalog/openngc directly.
	remote.EnableDownloads(5<<20, remote.OpenNGC) // ~2 MB combined source CSVs
	// Earth orientation, for where the site is on the turning Earth:
	// the IERS bulletin, which the remote/eop import above fetches.
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	ctx := context.Background()

	resolver := catalog.NewResolver(catalog.OpenNGC, catalog.SIMBAD, catalog.MAST)

	query := "Andromeda"
	show.Title("Name resolution", fmt.Sprintf("Search(%q) across OpenNGC, SIMBAD and MAST", query))

	// 3. Perform a fuzzy Search combining all endpoints.
	//
	// The error is checked rather than discarded, and that distinction is the
	// point: an empty result now means the catalogs were asked and matched
	// nothing, where an error means at least one could not be reached. They
	// used to be the same answer.
	results, err := resolver.Search(ctx, query)
	if err != nil {
		log.Fatalf("search: %v", err)
	}

	if len(results) == 0 {
		show.Note("", "No matches found.")
		return
	}

	// 4. The merged, ranked targets: one row each, however many catalogs
	// contributed to it.
	tab := show.NewTable(show.Right("#"), show.Left("Name"), show.Left("Kind"), show.Left("Catalog"),
		show.Right("RA"), show.Right("Dec"))

	for i, t := range results {
		ra, dec := "", ""
		if t.HasCoord {
			ra, dec = t.Coord.RA().HMSString(1), t.Coord.Dec().DMSString(0)
		}

		tab.Row(fmt.Sprint(i+1), t.Name, string(t.Kind), t.Catalog, ra, dec)
	}

	fmt.Println()
	tab.Print()

	// Up to three aliases each, which is why a row matched.
	show.Section("Why they matched")

	for i, t := range results {
		if len(t.Aliases) == 0 {
			continue
		}

		alts := t.Aliases[:min(len(t.Aliases), 3)]
		show.Note(fmt.Sprintf("%d. %s", i+1, strings.Join(alts, ", ")))
	}

	// 5. Resolve cross-matches every provider's hit for one name and merges
	// them into a single, richer Target, recording which catalog gave each
	// field.
	exact, err := resolver.Resolve(ctx, "M31")
	if err != nil {
		log.Fatalf("resolve: %v", err)
	}

	show.Section(`Resolve("M31")`)
	show.Fields(
		show.KV("Name", "%s", exact.Name),
		show.KV("ID", "%s", exact.ID),
		show.KV("Fields merged", "%d, each with the catalog it came from", len(exact.Provenance)),
	)
}
