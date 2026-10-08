package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestFromCatalogPlacesAStarFromItsOwnEpoch builds the same star from
// SIMBAD's row, at J2000, and from Gaia DR3's, at J2016.0, both read live on
// 2026-10-08 (#635). Built from either, it must be in the same place on the
// sky. Until #635 FromCatalog handed Gaia's 2016 position to a Star as
// though it were the 2000 one, and Barnard's star came out 166″ away.
func TestFromCatalogPlacesAStarFromItsOwnEpoch(t *testing.T) {
	t.Parallel()

	gaiaEpoch := time.FromJD(2457389.0, time.TT)

	for _, c := range []struct {
		name         string
		simbad, gaia resolve.Target
		maxArcsec    float64
	}{
		{
			name: "Barnard's star",
			simbad: resolve.Target{
				Name: "Barnard's star", Kind: resolve.KindStar, HasCoord: true, Epoch: time.J2000(),
				Coord: coord.NewICRS(angle.Deg(269.4520769586), angle.Deg(4.6933649666)),
				PmRA:  angle.Arcsec(-0.8015510), PmDec: angle.Arcsec(10.3623940), Parallax: angle.Arcsec(0.5469759),
				RadialVelocity: unit.KmPerSec(-110.11), HasRadialVelocity: true,
			},
			gaia: resolve.Target{
				Name: "Gaia DR3 4472832130942575872", Kind: resolve.KindStar, HasCoord: true, Epoch: gaiaEpoch,
				Coord: coord.NewICRS(angle.Deg(269.4485025254), angle.Deg(4.7394200511)),
				PmRA:  angle.Arcsec(-0.8015510), PmDec: angle.Arcsec(10.3623942), Parallax: angle.Arcsec(0.5469759),
			},
			// SIMBAD's row carries the star's −110 km/s radial velocity
			// and Gaia's none, and that much motion along the line of sight
			// is perspective acceleration: measured, 0.46″ by the 26 years to
			// the night below. The two rows describe the same motion
			// otherwise.
			maxArcsec: 0.5,
		},
		{
			name: "HD 209458",
			simbad: resolve.Target{
				Name: "HD 209458", Kind: resolve.KindStar, HasCoord: true, Epoch: time.J2000(),
				Coord: coord.NewICRS(angle.Deg(330.7948864439), angle.Deg(18.8843192759)),
				PmRA:  angle.Arcsec(0.0297660), PmDec: angle.Arcsec(-0.0179760), Parallax: angle.Arcsec(0.0207694),
				RadialVelocity: unit.KmPerSec(-14.743), HasRadialVelocity: true,
			},
			gaia: resolve.Target{
				Name: "Gaia DR3 1779546757669063552", Kind: resolve.KindStar, HasCoord: true, Epoch: gaiaEpoch,
				Coord: coord.NewICRS(angle.Deg(330.7950262642), angle.Deg(18.8842393829)),
				PmRA:  angle.Arcsec(0.0297664), PmDec: angle.Arcsec(-0.0179760), Parallax: angle.Arcsec(0.0207694),
			},
			maxArcsec: 0.01,
		},
	} {
		at := time.Date(2026, 10, 1, 4, 0, 0, 0, time.LocationUTC)

		var now [2]coord.ICRS

		for i, tg := range []resolve.Target{c.simbad, c.gaia} {
			obj, err := FromCatalog(tg, nil)
			if err != nil {
				t.Fatalf("%s: FromCatalog(%s): %v", c.name, tg.Name, err)
			}

			pos, err := obj.Position(at)
			if err != nil {
				t.Fatalf("%s: Position: %v", c.name, err)
			}

			// A Star's position is at J2000; move it to the night.
			if now[i], err = coord.PropagateEpoch(pos, time.J2000(), at); err != nil {
				t.Fatalf("%s: PropagateEpoch: %v", c.name, err)
			}
		}

		sep := coord.Separation(now[0], now[1]).Arcseconds()
		t.Logf("%s: built from SIMBAD and from Gaia, %.4f″ apart on 2026-10-01", c.name, sep)

		if sep > c.maxArcsec {
			t.Errorf("%s: %.3f″ apart, want under %g", c.name, sep, c.maxArcsec)
		}
	}
}

// TestFromCatalogLeavesAJ2000StarAsGiven: the common case, a catalog
// position at J2000 or with no epoch at all, is not moved.
func TestFromCatalogLeavesAJ2000StarAsGiven(t *testing.T) {
	t.Parallel()

	for _, epoch := range []time.Time{time.J2000(), {}} {
		tg := resolve.Target{
			Name: "Vega", Kind: resolve.KindStar, HasCoord: true, Epoch: epoch,
			Coord: coord.NewICRS(angle.Deg(279.23473479), angle.Deg(38.78368896)),
			PmRA:  angle.Arcsec(0.20094), PmDec: angle.Arcsec(0.28623), Parallax: angle.Arcsec(0.13023),
		}

		obj, err := FromCatalog(tg, nil)
		if err != nil {
			t.Fatal(err)
		}

		pos, err := obj.Position(time.J2000())
		if err != nil {
			t.Fatal(err)
		}

		if pos.RA() != tg.Coord.RA() || pos.Dec() != tg.Coord.Dec() {
			t.Errorf("epoch %v: Vega moved to (%v, %v)", epoch, pos.RA(), pos.Dec())
		}
	}
}
