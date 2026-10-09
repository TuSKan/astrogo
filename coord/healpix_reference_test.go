package coord_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// TestHEALPixMatchesAstropyHEALPix holds PixelOf to an independent NESTED
// implementation, astropy-healpix 2.0.1
// (HEALPix(nside, order='nested').lonlat_to_healpix), at nside 256 and 4096
// (#654).
//
// The other HEALPix tests check round trips, equal area, coverage and the
// hierarchy, which a scheme numbering its base faces another way, or
// interleaving x and y the other way round, passes too. The convention is
// what dataset/starlight relies on: it builds its map from Gaia, whose
// source_id carries the standard nested pixel, and reads it back through
// PixelOf.
//
// The points sit in each of the twelve base faces, near both poles and the
// 0/360 seam, and toward the Galactic center, all away from pixel boundaries.
// Exactly on a vertex, four pixels meet and implementations may break the tie
// differently, as these two do at (45°, 0°).
func TestHEALPixMatchesAstropyHEALPix(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		nside    int64
		lon, lat float64
		want     int64
	}{
		{256, 15.5, 62.3, 59437},
		{256, 105.2, 55.1, 113669},
		{256, 195.7, 48.9, 176838},
		{256, 285.4, 70.2, 257206},
		{256, 30.3, 10.1, 292255},
		{256, 120.6, -12.4, 651003},
		{256, 210.9, 5.7, 417486},
		{256, 300.2, -8.8, 476640},
		{256, 60.7, -55.3, 532468},
		{256, 150.1, -63.4, 597162},
		{256, 240.8, -48.2, 680341},
		{256, 330.4, -71.9, 727044},
		{256, 359.7, 0.3, 311298},
		{256, 77.3, 89.6, 65533},
		{256, 233.3, -89.4, 655363},
		{256, 266.4051, -28.936, 461282},
		{4096, 15.5, 62.3, 15215949},
		{4096, 105.2, 55.1, 29099343},
		{4096, 195.7, 48.9, 45270674},
		{4096, 285.4, 70.2, 65844932},
		{4096, 30.3, 10.1, 74817403},
		{4096, 120.6, -12.4, 166656986},
		{4096, 210.9, 5.7, 106876509},
		{4096, 300.2, -8.8, 122020042},
		{4096, 60.7, -55.3, 136311977},
		{4096, 150.1, -63.4, 152873657},
		{4096, 240.8, -48.2, 174167427},
		{4096, 330.4, -71.9, 186123311},
		{4096, 359.7, 0.3, 79692454},
		{4096, 77.3, 89.6, 16776519},
		{4096, 233.3, -89.4, 167773047},
		{4096, 266.4051, -28.936, 118088310},
	} {
		if got := healpix(t, c.nside).PixelOf(angle.Deg(c.lon), angle.Deg(c.lat)); got != c.want {
			t.Errorf("nside %d, (%g°, %g°): pixel %d, astropy-healpix %d", c.nside, c.lon, c.lat, got, c.want)
		}
	}
}
