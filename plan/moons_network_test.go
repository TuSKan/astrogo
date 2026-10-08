//go:build network

package plan

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// horizonsAbsoluteMagnitudeRe reads "V(1,0)= -1.28" from Horizons' physical
// data block.
var horizonsAbsoluteMagnitudeRe = regexp.MustCompile(`V\(1,0\)\s*=\s*([-+0-9.]+)`)

// TestMoonAbsoluteMagnitudesAreHorizons holds every moon moonSpecs documents
// as Horizons-sourced to Horizons' own V(1,0) (#638). The Galilean moons and
// Charon are the documented exceptions: Horizons publishes no V(1,0) for
// them. Four of the sixteen had drifted from Horizons by up to 0.17 mag
// (Hyperion) with nothing to notice.
func TestMoonAbsoluteMagnitudesAreHorizons(t *testing.T) {
	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")

	exceptions := map[string]bool{"io": true, "europa": true, "ganymede": true, "callisto": true, "charon": true}

	for key, m := range moonSpecs {
		if exceptions[key] {
			continue
		}

		t.Run(m.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			params := url.Values{}
			params.Set("format", "json")
			params.Set("COMMAND", fmt.Sprintf("'%d'", m.naifID))
			params.Set("OBJ_DATA", "YES")
			params.Set("MAKE_EPHEM", "NO")

			var payload struct {
				Result string `json:"result"`
			}

			if err := remote.Default().GetJSON(ctx, remote.JPLHorizons, "", params, &payload); err != nil {
				testutil.SkipOnUpstreamFailure(t, err)
				t.Fatalf("Horizons %d: %v", m.naifID, err)
			}

			match := horizonsAbsoluteMagnitudeRe.FindStringSubmatch(payload.Result)
			if match == nil {
				t.Fatalf("Horizons publishes no V(1,0) for %s (%d)", m.name, m.naifID)
			}

			want, err := strconv.ParseFloat(match[1], 64)
			if err != nil {
				t.Fatalf("Horizons V(1,0) %q: %v", match[1], err)
			}

			// Horizons prints two decimals at most, and the table carries
			// the value as printed.
			if math.Abs(m.h-want) > 0.005 {
				t.Errorf("%s: H = %g, Horizons V(1,0) = %g", m.name, m.h, want)
			}
		})
	}
}
