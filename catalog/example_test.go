package catalog_test

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/resolve"
)

// The task this package exists for: turn a name someone typed into a target
// with coordinates, by asking the catalogues that might know.
//
// The three-way switch below is the part worth copying. Resolve returning a
// typed sentinel rather than a bare error is what lets a caller tell "the
// catalogues answered, and the answer is no" from "the catalogues could not be
// reached" — and treating the second as the first turns an outage into a
// silently empty sky.
//
// The query here is one no catalogue can resolve, which keeps the example
// offline and therefore executable. A real one — "M31" — takes exactly the same
// path.
func Example() {
	resolver := catalog.NewResolver(catalog.SIMBAD)

	target, err := resolver.Resolve(context.Background(), "")

	switch {
	case errors.Is(err, resolve.ErrNotFound):
		// Answered, and the answer is no. An ordinary negative.
		fmt.Println("no such object")

	case errors.Is(err, resolve.ErrAmbiguous):
		// Answered, and the answer is "which one?". Narrow the query.
		fmt.Println("name matches several objects")

	case err != nil:
		// NOT an answer. Unreachable service, malformed response, cancelled
		// context. This is the branch that must not be folded into the first.
		fmt.Println("could not ask:", err)

	default:
		fmt.Printf("%s at RA %.4f°, Dec %.4f°\n",
			target.Name, target.Coord.RA().Degrees(), target.Coord.Dec().Degrees())
	}

	// Output:
	// no such object
}

// What a provider can do beyond resolving a name is an optional interface
// rather than a method on [catalog.Provider], so asking is a type assertion.
// That is how a caller finds out up front instead of by way of ErrUnsupported.
//
// Constructing a provider and interrogating its interfaces is entirely local,
// so this runs offline too; only the queries themselves need a network.
func ExampleNewProvider() {
	provider, err := catalog.NewProvider(catalog.SIMBAD)
	if err != nil {
		log.Fatalf("NewProvider: %v", err)
	}

	fmt.Println("provider:", provider.Name())

	_, cone := provider.(resolve.ConeSearcher)
	_, bright := provider.(resolve.BrightObjectSearcher)

	fmt.Println("cone search:  ", cone)
	fmt.Println("bright browse:", bright)

	// Output:
	// provider: simbad
	// cone search:   false
	// bright browse: true
}
