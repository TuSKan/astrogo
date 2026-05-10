// Package main demonstrates astronomical name resolution.
package main

import (
	"fmt"

	"github.com/TuSKan/astrogo/catalog"
)

func main() {
	resolver := catalog.NewResolver(catalog.OpenNGC, catalog.SIMBAD, catalog.MAST)

	query := "Andromeda"
	fmt.Printf("Executing advanced Search for ambiguous query: %q...\n", query)

	// 3. Perform a fuzzy Search combining all endpoints
	results := resolver.Search(query)

	if len(results) == 0 {
		fmt.Println("No matches found.")
		return
	}

	fmt.Printf("\nFound %d matching objects across the catalogs:\n\n", len(results))

	// 4. Iterate over the uniquely merged and ranked targets
	for i, t := range results {
		fmt.Printf("[%d] %-30s | Kind: %-15s | DB: %s\n", i+1, t.Name, t.Kind, t.Catalog)
		fmt.Printf("    ID: %-26s | ICRS: %s\n", t.ID, t.Coord)

		if len(t.Aliases) > 0 {
			// Show up to 3 aliases to demonstrate why it matched
			alts := t.Aliases
			if len(alts) > 3 {
				alts = alts[:3]
			}

			fmt.Printf("    Aliases: %v\n", alts)
		}

		fmt.Println("    -------------------------------------------------------------------")
	}

	// 5. Demonstrate the strict Resolve() guarantee
	fmt.Printf("\nExecuting strict Resolve() on %q...\n", "M31")

	exact, err := resolver.Resolve("M31")
	if err == nil {
		fmt.Printf("Strictly matched: %s (%s) perfectly.\n", exact.Name, exact.ID)
	}
}
