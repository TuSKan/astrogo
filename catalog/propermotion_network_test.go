//go:build network

package catalog

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// TestResolverMergesGaiaForMovingStarsLive is #627 against the live
// services: a SIMBAD star's Gaia DR3 row is merged into its group however
// fast the star moves. Before #627 none of these but HD 209458, at 35
// mas/yr, took a field from Gaia.
func TestResolverMergesGaiaForMovingStarsLive(t *testing.T) {
	testutil.RequireReachable(t, "simbad.cds.unistra.fr:80")
	testutil.RequireReachable(t, "gea.esac.esa.int:443")

	r := NewResolver(SIMBAD, Gaia)

	for _, star := range []string{"HD 209458", "HD 189733", "tau Cet", "Barnard's star"} {
		t.Run(star, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			got, err := r.Resolve(ctx, star)
			if err != nil {
				testutil.SkipOnUpstreamFailure(t, err)
				t.Fatalf("Resolve: %v", err)
			}

			if got.Provenance["Parallax"] != "gaia" && got.Provenance["Coord"] != "gaia" && got.Provenance["PmRA"] != "gaia" {
				t.Errorf("no field came from Gaia; provenance %v", got.Provenance)
			}
		})
	}
}
