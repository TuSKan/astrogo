package catalog

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// The rows below are SIMBAD's, Gaia DR3's and 2MASS's, read live on
// 2026-10-08 through this module's own providers (#627). SIMBAD's positions
// are at J2000 and Gaia DR3's at J2016.0; 2MASS observed Barnard's star on
// JD 2451692.9284, 2000 May 28.

var gaiaDR3EpochForTest = time.FromJD(2457389.0, time.TT)

// simbadRow is a Target as catalog/simbad builds one: RA and Dec in Coord,
// the motion in the Target's own fields.
func simbadRow(id string, ra, dec, pmRAmas, pmDecmas, plxmas, rvkms float64) Target {
	return Target{
		ID: id, Name: id, Catalog: "SIMBAD", Kind: resolve.KindStar,
		Coord: coord.NewICRS(angle.Deg(ra), angle.Deg(dec)), HasCoord: true, Epoch: time.J2000(),
		PmRA: angle.Arcsec(pmRAmas / 1000), PmDec: angle.Arcsec(pmDecmas / 1000),
		Parallax: angle.Arcsec(plxmas / 1000), RadialVelocity: unit.KmPerSec(rvkms), HasRadialVelocity: true,
	}
}

// gaiaRow is a Target as catalog/gaia builds one.
func gaiaRow(id string, ra, dec, pmRAmas, pmDecmas, plxmas float64) Target {
	return Target{
		ID: id, Name: "Gaia DR3 " + id, Catalog: "Gaia DR3", Kind: resolve.KindStar,
		Coord: coord.NewICRS(angle.Deg(ra), angle.Deg(dec)), HasCoord: true, Epoch: gaiaDR3EpochForTest,
		PmRA: angle.Arcsec(pmRAmas / 1000), PmDec: angle.Arcsec(pmDecmas / 1000), Parallax: angle.Arcsec(plxmas / 1000),
	}
}

var (
	simbadBarnard = simbadRow("NAME Barnard's star", 269.4520769586, 4.6933649666, -801.5510, 10362.3940, 546.9759, -110.11)
	gaiaBarnard   = gaiaRow("4472832130942575872", 269.4485025254, 4.7394200511, -801.5510, 10362.3942, 546.9759)
	simbadHD189   = simbadRow("HD 189733", 300.1821372640, 22.7108536510, -3.2080, -250.3230, 50.5668, -2.317)
	gaiaHD189     = gaiaRow("1827242816201846144", 300.1821218062, 22.7097411052, -3.2083, -250.3233, 50.5668)
	simbadHD209   = simbadRow("HD 209458", 330.7948864439, 18.8843192759, 29.7660, -17.9760, 20.7694, -14.743)
	gaiaHD209     = gaiaRow("1779546757669063552", 330.7950262642, 18.8842393829, 29.7664, -17.9760, 20.7694)
)

// TestSeparationAtCommonEpoch covers its three cases on real rows.
func TestSeparationAtCommonEpoch(t *testing.T) {
	t.Parallel()

	// 2MASS publishes no motion; its row is where the star was on the night.
	twoMASS := coord.NewICRS(angle.Deg(269.452044), angle.Deg(4.694597))
	twoMASSEpoch := time.FromJD(2451692.9284, time.TT)

	for _, c := range []struct {
		name       string
		a          coord.ICRS
		aEpoch     time.Time
		b          coord.ICRS
		bEpoch     time.Time
		maxArcsec  float64
		minArcsec  float64
		whyMeasure string
	}{
		{
			name: "both moving: SIMBAD at J2000, Gaia at J2016",
			a:    kinematicCoord(simbadBarnard), aEpoch: simbadBarnard.Epoch,
			b: kinematicCoord(gaiaBarnard), bEpoch: gaiaBarnard.Epoch,
			maxArcsec: 0.01, whyMeasure: "166″ apart as published",
		},
		{
			name: "one moving: SIMBAD moved to the night 2MASS observed",
			a:    kinematicCoord(simbadBarnard), aEpoch: simbadBarnard.Epoch,
			b: twoMASS, bEpoch: twoMASSEpoch,
			maxArcsec: 0.5, whyMeasure: "4.4″ apart as published",
		},
		{
			name: "neither moving: compared as given",
			a:    simbadBarnard.Coord, aEpoch: simbadBarnard.Epoch,
			b: twoMASS, bEpoch: twoMASSEpoch,
			minArcsec: 4.4, maxArcsec: 4.5, whyMeasure: "nothing to move",
		},
	} {
		sep, err := separationAtCommonEpoch(c.a, c.aEpoch, c.b, c.bEpoch)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		t.Logf("%s: %.3f″ (%s)", c.name, sep.Arcseconds(), c.whyMeasure)

		if s := sep.Arcseconds(); s > c.maxArcsec || s < c.minArcsec {
			t.Errorf("%s: %.3f″, want %g to %g", c.name, s, c.minArcsec, c.maxArcsec)
		}
	}
}

// TestResolverMatchesAMovingStarsGaiaRow: the Resolver's bridge folds a
// star's Gaia row into its group only within the 2″ threshold, and until
// #627 it compared SIMBAD's J2000 position with Gaia's J2016 one unmoved,
// because the motion was never put into the position it propagated. A
// star faster than about 0.125″/yr could not match; HD 209458, at 35
// mas/yr, could and is the control.
func TestResolverMatchesAMovingStarsGaiaRow(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		simbad, gaia Target
	}{
		{simbadBarnard, gaiaBarnard},
		{simbadHD189, gaiaHD189},
		{simbadHD209, gaiaHD209},
	} {
		r := &Resolver{
			providers:     []Provider{&mockProvider{name: "simbad", targets: map[string]Target{resolve.Normalize(c.simbad.ID): c.simbad}}},
			coneSearchers: []coneProvider{{name: "gaia", cs: &mockConeSearcher{targets: []Target{c.gaia}}}},
			cfg:           resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
		}

		got, err := r.Resolve(context.Background(), c.simbad.ID)
		if err != nil {
			t.Fatalf("%s: %v", c.simbad.ID, err)
		}

		if got.Provenance["Parallax"] != "gaia" && got.Provenance["Coord"] != "gaia" && got.Provenance["PmRA"] != "gaia" {
			t.Errorf("%s: no field came from its Gaia row %s; provenance %v", c.simbad.ID, c.gaia.ID, got.Provenance)
		}
	}
}

// TestResolverMatchesAStarsTwoMASSRowAtItsOwnDate: catalog/vizier stamps a
// 2MASS row with the night 2MASS observed it (#628), where every row used to
// carry J2000. Barnard's star from SIMBAD, moved to that night, is 0.3″ from
// its row and folds it into the group; moved to the J2000 the row used to
// carry, it is 4.4″ away, outside the 2″ match, and the row stays out. The
// merged epoch is SIMBAD's, the epoch of the position that won, not the row's.
func TestResolverMatchesAStarsTwoMASSRowAtItsOwnDate(t *testing.T) {
	t.Parallel()

	row := func(epoch time.Time) Target {
		return Target{
			ID: "17574849+0441405", Name: "17574849+0441405", Catalog: "vizier", Kind: resolve.KindStar,
			Coord: coord.NewICRS(angle.Deg(269.452044), angle.Deg(4.694597)), HasCoord: true, Epoch: epoch,
		}
	}

	for _, c := range []struct {
		name    string
		epoch   time.Time
		matches bool
	}{
		{"stamped with its own date", time.FromJD(2451692.9284, time.UTC), true},
		{"stamped J2000, as every row used to be", time.J2000(), false},
	} {
		r := &Resolver{
			providers: []Provider{&mockProvider{name: "simbad", targets: map[string]Target{
				resolve.Normalize(simbadBarnard.ID): simbadBarnard,
			}}},
			coneSearchers: []coneProvider{{name: "vizier", cs: &mockConeSearcher{targets: []Target{row(c.epoch)}}}},
			cfg:           resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
		}

		got, err := r.Resolve(context.Background(), simbadBarnard.ID)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if matched := containsNormalized(got.Aliases, "17574849+0441405"); matched != c.matches {
			t.Errorf("%s: 2MASS row in the group = %v, want %v (aliases %v)", c.name, matched, c.matches, got.Aliases)
		}

		if got.Provenance["Epoch"] != "simbad" || !got.Epoch.Equal(time.J2000()) {
			t.Errorf("%s: Epoch = %v from %q, want SIMBAD's J2000, the epoch of the Coord it chose",
				c.name, got.Epoch, got.Provenance["Epoch"])
		}
	}
}
