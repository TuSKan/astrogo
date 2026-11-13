package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// issGP is CelesTrak's GP answer for the ISS, in the JSON form catalog/norad
// requests.
const issGP = `[{
    "OBJECT_NAME": "ISS (ZARYA)",
    "OBJECT_ID": "1998-067A",
    "EPOCH": "2026-04-19T11:45:32.833440",
    "MEAN_MOTION": 15.4883325,
    "ECCENTRICITY": 0.00066312,
    "INCLINATION": 51.6329,
    "RA_OF_ASC_NODE": 230.6068,
    "ARG_OF_PERICENTER": 325.6576,
    "MEAN_ANOMALY": 34.3983,
    "EPHEMERIS_TYPE": 0,
    "CLASSIFICATION_TYPE": "U",
    "NORAD_CAT_ID": 25544,
    "ELEMENT_SET_NO": 999,
    "REV_AT_EPOCH": 56265,
    "BSTAR": 0.00019193879,
    "MEAN_MOTION_DOT": 0.00010082,
    "MEAN_MOTION_DDOT": 0
}]`

// TestResolverKeepsTheTLEEpoch runs the real NORAD provider through a
// Resolver. The provider sets Epoch from the GP epoch, and the merge used to
// drop it while keeping the TLE lines, so a resolved satellite came back with
// the zero Time and SGP4 propagated it from JD 0 without an error (#1130).
func TestResolverKeepsTheTLEEpoch(t *testing.T) {
	t.Cleanup(remote.Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(issGP))
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.CelesTrak, srv.URL); err != nil {
		t.Fatal(err)
	}

	want := time.Date(2026, 4, 19, 11, 45, 32, 833440000, time.LocationUTC)
	r := NewResolver(NORAD)

	resolved, err := r.Resolve(context.Background(), "ISS")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	searched, err := r.Search(context.Background(), "ISS")
	if err != nil || len(searched) == 0 {
		t.Fatalf("Search = %d targets, %v; want the ISS", len(searched), err)
	}

	for name, got := range map[string]Target{"Resolve": resolved, "Search": searched[0]} {
		if got.TLELine1 == "" || got.TLELine2 == "" {
			t.Fatalf("%s lost the TLE: %q, %q", name, got.TLELine1, got.TLELine2)
		}

		if !got.Epoch.Equal(want) {
			t.Errorf("%s Epoch = %v (JD %.6f), want the GP epoch %v (JD %.6f)",
				name, got.Epoch, got.Epoch.JD(), want, want.JD())
		}

		if got.Provenance["Epoch"] != "norad" {
			t.Errorf("%s Epoch provenance = %q, want norad", name, got.Provenance["Epoch"])
		}
	}
}

// TestMergeGroupTakesTheTLEFromOneCandidate: the two lines and their epoch
// are one element set. The lines used to be copied independently, so a group
// whose first candidate had only line 1 paired it with another candidate's
// line 2.
func TestMergeGroupTakesTheTLEFromOneCandidate(t *testing.T) {
	first := time.Date(2026, 4, 18, 0, 0, 0, 0, time.LocationUTC)
	second := time.Date(2026, 4, 19, 0, 0, 0, 0, time.LocationUTC)

	got := mergeGroup(group{candidates: []candidate{
		{provider: "norad", target: Target{ID: "25544", TLELine1: "first line 1", Epoch: first}},
		{provider: "norad", target: Target{ID: "25544", TLELine1: "second line 1", TLELine2: "second line 2", Epoch: second}},
	}})

	if got.TLELine1 != "second line 1" || got.TLELine2 != "second line 2" {
		t.Errorf("TLE = %q, %q; want both lines of the complete element set", got.TLELine1, got.TLELine2)
	}

	if !got.Epoch.Equal(second) {
		t.Errorf("Epoch = %v, want %v, the epoch of the element set taken", got.Epoch, second)
	}
}

// TestMergeGroupTLEEpochWinsOverAPositionEpoch: where a group also holds a
// catalog position, the TLE keeps its own epoch, as SBDB's elements do,
// since SGP4 reads Epoch as the element set's.
func TestMergeGroupTLEEpochWinsOverAPositionEpoch(t *testing.T) {
	tleEpoch := time.Date(2026, 4, 19, 11, 45, 32, 0, time.LocationUTC)

	got := mergeGroup(group{candidates: []candidate{
		{provider: "simbad", target: Target{
			ID: "shared", Coord: coord.NewICRS(angle.Deg(10), angle.Deg(20)), HasCoord: true, Epoch: time.J2000(),
		}},
		{provider: "norad", target: Target{ID: "shared", TLELine1: "line 1", TLELine2: "line 2", Epoch: tleEpoch}},
	}})

	if !got.Epoch.Equal(tleEpoch) || got.Provenance["Epoch"] != "norad" {
		t.Errorf("Epoch = %v from %q, want the TLE's %v from norad", got.Epoch, got.Provenance["Epoch"], tleEpoch)
	}

	if !got.HasCoord || got.Provenance["Coord"] != "simbad" {
		t.Errorf("Coord from %q (has=%v), want SIMBAD's position kept", got.Provenance["Coord"], got.HasCoord)
	}
}
