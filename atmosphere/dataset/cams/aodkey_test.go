package cams_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere/dataset/cams"
	"github.com/TuSKan/astrogo/time"
)

// An instant resolves to the most recent CAMS cycle at or before it, and to
// the forecast step that lands on it.
//
// # Why the rule is "at or before" rather than "nearest"
//
// CAMS runs at 00Z and 12Z and publishes hourly steps from each, so most
// hours are covered by both the current cycle at a long lead time and the
// next cycle at a negative one — which does not exist. Taking the nearest
// cycle would ask for a forecast issued after the hour it describes. Taking
// the most recent one at or before also means every hour is served by the
// freshest run that could have covered it, which is the more accurate of the
// two available answers.
//
// The boundaries are where a rule like this goes wrong, so they are the cases
// here: 11:59 must still belong to the 00Z cycle at step 11, and 12:00 must
// have already moved to the 12Z cycle at step 0.
func TestAODKeySelectsTheCycleAtOrBeforeTheInstant(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name  string
		when  time.GoTime
		cycle string
		step  string
	}{
		{
			"morning, 00Z cycle",
			time.GoDate(2026, time.March, 20, 5, 0, 0, 0, time.LocationUTC),
			"20260320000000", "005",
		},
		{
			"last hour before the switch",
			time.GoDate(2026, time.March, 20, 11, 59, 0, 0, time.LocationUTC),
			"20260320000000", "011",
		},
		{
			"exactly the switch",
			time.GoDate(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC),
			"20260320120000", "000",
		},
		{
			"evening, 12Z cycle, part-hour truncates down",
			time.GoDate(2026, time.March, 20, 18, 30, 0, 0, time.LocationUTC),
			"20260320120000", "006",
		},
		{
			"last hour of the day",
			time.GoDate(2026, time.March, 20, 23, 0, 0, 0, time.LocationUTC),
			"20260320120000", "011",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := cams.AODKey(c.when)

			want := "CAMS/GLOBAL/2026/03/20/z_cams_c_ecmf_" + c.cycle +
				"_prod_fc_sfc_" + c.step + "_aod550/z_cams_c_ecmf_" + c.cycle +
				"_prod_fc_sfc_" + c.step + "_aod550.nc"

			if got != want {
				t.Errorf("AODKey(%s)\n got %q\nwant %q", c.when.Format(time.RFC3339), got, want)
			}
		})
	}
}

// A non-UTC instant is converted rather than read as though its wall clock
// were UTC.
//
// Worth its own case because the failure is invisible: a caller in Chile
// passing a local 05:00 would silently get the 08:00 UTC file, which exists,
// parses, and describes a different three hours of atmosphere.
func TestAODKeyConvertsToUTC(t *testing.T) {
	t.Parallel()

	east := time.FixedZone("UTC+2", 2*60*60)

	// 05:00+02:00 is 03:00 UTC, so step 3 of the 00Z cycle.
	got := cams.AODKey(time.GoDate(2026, time.March, 20, 5, 0, 0, 0, east))

	if !strings.Contains(got, "_prod_fc_sfc_003_") {
		t.Errorf("AODKey for 05:00+02:00 is %q; 03:00 UTC is step 3 of the 00Z cycle", got)
	}

	// And the same instant expressed in UTC gives byte-identical output.
	if utc := cams.AODKey(time.GoDate(2026, time.March, 20, 3, 0, 0, 0, time.LocationUTC)); utc != got {
		t.Errorf("the same instant in two zones gave different keys:\n%q\n%q", got, utc)
	}
}

// The key names the variable this package reads, and the directory repeats
// the file stem.
//
// Both are properties of the CAMS layout on the Copernicus store rather than
// choices this package makes, so they are pinned: a caller debugging a 404
// needs to know whether the key was built wrong or the object is genuinely
// absent.
func TestAODKeyShapeMatchesTheStoreLayout(t *testing.T) {
	t.Parallel()

	key := cams.AODKey(time.GoDate(2026, time.January, 2, 7, 0, 0, 0, time.LocationUTC))

	if !strings.HasPrefix(key, "CAMS/GLOBAL/2026/01/02/") {
		t.Errorf("key %q does not start with the zero-padded date prefix", key)
	}

	if !strings.HasSuffix(key, ".nc") {
		t.Errorf("key %q is not a NetCDF object", key)
	}

	if !strings.Contains(key, cams.AODVariable) {
		t.Errorf("key %q does not name %q, the variable this package reads",
			key, cams.AODVariable)
	}

	// The directory and the file share a stem: .../<name>/<name>.nc
	parts := strings.Split(strings.TrimSuffix(key, ".nc"), "/")
	if len(parts) < 2 || parts[len(parts)-1] != parts[len(parts)-2] {
		t.Errorf("key %q does not repeat the file stem as its directory", key)
	}
}
