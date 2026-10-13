package xmatch

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// The rows below are the ones catalog's own proper-motion tests use (#627):
// SIMBAD's, Gaia DR3's and 2MASS's, read live on 2026-10-08 through this
// module's providers. SIMBAD's positions are at J2000 and Gaia DR3's at
// J2016.0; 2MASS observed Barnard's star on JD 2451692.9284, 2000 May 28,
// and publishes no motion.
//
// Each is built the way its provider builds it: RA and Dec in Coord, the
// motion in the Target's own fields. That is the whole point of the test —
// a propagation that reads the motion from Coord moves nothing.

var gaiaDR3Epoch = time.FromJD(2457389.0, time.TT)

func simbadRow(id string, ra, dec, pmRAmas, pmDecmas, plxmas, rvkms float64) resolve.Target {
	return resolve.Target{
		ID: id, Name: id, Catalog: "SIMBAD", Kind: resolve.KindStar,
		Coord: coord.NewICRS(angle.Deg(ra), angle.Deg(dec)), HasCoord: true, Epoch: time.J2000(),
		PmRA: angle.Arcsec(pmRAmas / 1000), PmDec: angle.Arcsec(pmDecmas / 1000),
		Parallax: angle.Arcsec(plxmas / 1000), RadialVelocity: unit.KmPerSec(rvkms), HasRadialVelocity: true,
	}
}

func gaiaRow(id string, ra, dec, pmRAmas, pmDecmas, plxmas float64) resolve.Target {
	return resolve.Target{
		ID: id, Name: "Gaia DR3 " + id, Catalog: "Gaia DR3", Kind: resolve.KindStar,
		Coord: coord.NewICRS(angle.Deg(ra), angle.Deg(dec)), HasCoord: true, Epoch: gaiaDR3Epoch,
		PmRA: angle.Arcsec(pmRAmas / 1000), PmDec: angle.Arcsec(pmDecmas / 1000), Parallax: angle.Arcsec(plxmas / 1000),
	}
}

// TestMatchAppliesTheTargetsOwnProperMotion: Match documents that it
// epoch-normalizes each Target before comparing positions, and until this
// change it did so through coord.PropagateEpoch(t.Coord, ...), which reads
// the motion from Coord, where no provider puts it. Nothing moved, so a star
// faster than about 0.125″/yr could not match its own Gaia row: Barnard's
// star is 166″ from it as published. HD 209458, at 35 mas/yr, is close
// enough to match unmoved and is the control.
//
// The 2MASS case is the other half. A row that publishes no motion cannot be
// moved, so it has to be compared where the star was on the night it was
// observed — 0.3″ from SIMBAD's row moved there — rather than with SIMBAD's
// J2000 position, which is 4.4″ away.
func TestMatchAppliesTheTargetsOwnProperMotion(t *testing.T) {
	t.Parallel()

	twoMASSBarnard := resolve.Target{
		ID: "2MASS J17574849+0441405", Name: "2MASS J17574849+0441405", Catalog: "2MASS", Kind: resolve.KindStar,
		Coord:    coord.NewICRS(angle.Deg(269.452044), angle.Deg(4.694597)),
		HasCoord: true, Epoch: time.FromJD(2451692.9284, time.TT),
	}

	for _, c := range []struct {
		name string
		a, b resolve.Target
	}{
		{
			"Barnard's star, SIMBAD against Gaia DR3",
			simbadRow("NAME Barnard's star", 269.4520769586, 4.6933649666, -801.5510, 10362.3940, 546.9759, -110.11),
			gaiaRow("4472832130942575872", 269.4485025254, 4.7394200511, -801.5510, 10362.3942, 546.9759),
		},
		{
			"HD 189733, SIMBAD against Gaia DR3",
			simbadRow("HD 189733", 300.1821372640, 22.7108536510, -3.2080, -250.3230, 50.5668, -2.317),
			gaiaRow("1827242816201846144", 300.1821218062, 22.7097411052, -3.2083, -250.3233, 50.5668),
		},
		{
			"HD 209458, SIMBAD against Gaia DR3 (control)",
			simbadRow("HD 209458", 330.7948864439, 18.8843192759, 29.7660, -17.9760, 20.7694, -14.743),
			gaiaRow("1779546757669063552", 330.7950262642, 18.8842393829, 29.7664, -17.9760, 20.7694),
		},
		{
			"Barnard's star, SIMBAD against 2MASS, which publishes no motion",
			simbadRow("NAME Barnard's star", 269.4520769586, 4.6933649666, -801.5510, 10362.3940, 546.9759, -110.11),
			twoMASSBarnard,
		},
	} {
		pairs := Match([]resolve.Target{c.a}, []resolve.Target{c.b})
		if len(pairs) != 1 {
			t.Errorf("%s: %d pairs, want 1", c.name, len(pairs))
		}
	}
}
