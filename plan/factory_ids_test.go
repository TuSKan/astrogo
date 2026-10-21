package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
)

// TestFromCatalogNamesSatellitesByTheirNAIFCode: a NORAD target's ID is its
// catalog number, which names it as NAIF does, −100000 − n. It used to become
// eph.ID(25544), a body no satellite provider answers for, so every position
// of a catalog-built ISS failed (#809).
func TestFromCatalogNamesSatellitesByTheirNAIFCode(t *testing.T) {
	t.Parallel()

	prov, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	obj, err := FromCatalog(resolve.Target{Name: "ISS", ID: "25544", Kind: resolve.KindSatellite, Catalog: "norad"}, prov)
	if err != nil {
		t.Fatalf("FromCatalog: %v", err)
	}

	sat, ok := obj.(*Satellite)
	if !ok {
		t.Fatalf("FromCatalog returned %T, want *Satellite", obj)
	}

	if sat.EphID() != prov.ID() {
		t.Errorf("EphID() = %v, want %v", sat.EphID(), prov.ID())
	}

	if _, err := sat.Position(time.Date(2026, 4, 20, 0, 0, 0, 0, time.LocationUTC)); err != nil {
		t.Errorf("Position: %v", err)
	}
}

// TestFromCatalogNamesNumberedSmallBodiesByTheirNAIFCode: a catalog that gives
// an asteroid by its number — FINK's 433, MPCORB's packed 00433 — names NAIF's
// 20000433, as an SPK-ID already does. Read as a NAIF code, 433 named no body
// and 301 would have been the Moon; a kernel lookup used to retry a missing
// code as a small body's, and no longer does (#809).
func TestFromCatalogNamesNumberedSmallBodiesByTheirNAIFCode(t *testing.T) {
	t.Parallel()

	eros, _ := eph.SmallBodyID(433)

	for _, c := range []resolve.Target{
		{Name: "433 Eros", ID: "433", Kind: resolve.KindAsteroid, Catalog: "fink", HasH: true, H: 10.4},
		{Name: "Eros", ID: "00433", Kind: resolve.KindAsteroid, Catalog: "mpcorb", HasH: true, H: 10.4},
		{Name: "433 Eros (A898 PA)", ID: "20000433", SPKID: "20000433", Kind: resolve.KindAsteroid, Catalog: "sbdb", HasH: true, H: 10.4},
	} {
		obj, err := FromCatalog(c, eph.Default())
		if err != nil {
			t.Fatalf("%s (%s): %v", c.Catalog, c.ID, err)
		}

		a, ok := obj.(*Asteroid)
		if !ok {
			t.Fatalf("%s (%s): got %T, want *Asteroid", c.Catalog, c.ID, obj)
		}

		if a.EphID() != eros {
			t.Errorf("%s (%s): EphID() = %v, want %v", c.Catalog, c.ID, a.EphID(), eros)
		}
	}
}
