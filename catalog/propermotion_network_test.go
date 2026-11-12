//go:build network

package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// simbadAndGaiaControl fails when SIMBAD or Gaia rejects a request a working
// service cannot: one main_id from SIMBAD's basic, one source_id from Gaia
// DR3, the columns each has served for as long as it has had a TAP service.
// The resolver below needs both, and its error does not say which failed, so
// either one failing its control makes the error the service's, not ours.
// The reachability checks miss an outage that still accepts connections, as
// SIMBAD's whole web application answering 404 did on 2026-11-10 (#1026).
//
// Its deadline starts when it runs, not when the test did.
func simbadAndGaiaControl(t *testing.T) func() error {
	t.Helper()

	type service struct {
		syncURL, from, column string
	}

	services := make([]service, 0, 2)

	for _, svc := range []struct {
		id           remote.EndpointID
		from, column string
	}{
		{remote.SIMBAD, "basic", "main_id"},
		{remote.GaiaTAP, "gaiadr3.gaia_source", "source_id"},
	} {
		syncURL, err := remote.URL(svc.id)
		if err != nil {
			t.Fatalf("%s endpoint: %v", svc.id, err)
		}

		services = append(services, service{syncURL, svc.from, svc.column})
	}

	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		errs := make([]error, 0, len(services))
		for _, svc := range services {
			errs = append(errs, testutil.TAPControl(ctx, svc.syncURL, svc.from, svc.column)())
		}

		return errors.Join(errs...)
	}
}

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
				testutil.SkipOnDegradedService(t, err, simbadAndGaiaControl(t))
				t.Fatalf("Resolve: %v", err)
			}

			if got.Provenance["Parallax"] != "gaia" && got.Provenance["Coord"] != "gaia" && got.Provenance["PmRA"] != "gaia" {
				t.Errorf("no field came from Gaia; provenance %v", got.Provenance)
			}
		})
	}
}
