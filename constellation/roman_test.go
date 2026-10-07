package constellation_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constellation"
	"github.com/TuSKan/astrogo/coord"
)

// romanBoundaryPoints are ICRS positions within about 22" of a constellation
// boundary, each with the constellation Roman (1987)'s boundary table gives
// once the position is precessed to B1875.0, and the answer astropy 8.0.1's
// get_constellation gives instead.
//
// They are the boundary-hugging subset of 200,000 uniformly random positions
// (seed 20261006): those where the two answers differ, kept only if Roman's
// answer survives a 2" shift in either coordinate, so that no difference
// between precession models can decide them. The reference is Roman's table
// as astropy ships it, constellation_data_roman87.dat, applied to
// FK5(equinox=B1875) coordinates, which are precession only.
//
// astropy's own get_constellation is not that. It precesses through
// PrecessedGeocentric(equinox="B1875"), a GCRS frame, so annual aberration
// and light deflection come along and move every position 18-22" from its
// mean B1875 place. Delporte's boundaries are mean B1875 coordinates and
// Roman's method precesses mean positions, so at every point here astropy's
// answer is the wrong one. A test written against get_constellation would
// fail astrogo on positions it gets right.
var romanBoundaryPoints = []struct {
	raDeg, decDeg float64
	roman         string
	astropy       string
}{
	{5.926864, -39.303570, "Scl", "Phe"},
	{22.444495, 53.859434, "Cas", "Per"},
	{25.747440, -50.867506, "Phe", "Eri"},
	{30.808097, -57.898189, "Eri", "Hyi"},
	{36.242702, -40.890254, "Phe", "Eri"},
	{37.315426, -51.691146, "Eri", "Hor"},
	{39.282532, 31.211336, "Ari", "Tri"},
	{54.587414, 72.509582, "Cas", "Cam"},
	{57.216687, 79.432620, "Cep", "Cam"},
	{60.757947, -53.989370, "Ret", "Dor"},
	{65.692050, -67.212127, "Ret", "Hyi"},
	{68.323134, -50.066604, "Dor", "Pic"},
	{68.834823, -66.628626, "Ret", "Dor"},
	{70.859450, 1.204737, "Tau", "Ori"},
	{71.739186, -28.372730, "Eri", "Cae"},
	{71.924697, -69.784125, "Dor", "Men"},
	{72.547392, 41.173321, "Per", "Aur"},
	{76.217758, -29.340555, "Cae", "Col"},
	{77.532389, 54.231509, "Cam", "Aur"},
	{77.745080, -8.516931, "Eri", "Ori"},
	{85.787339, 15.281913, "Ori", "Tau"},
	{92.935030, -30.858011, "Col", "CMa"},
	{92.966935, -28.711576, "Col", "CMa"},
	{98.043218, -56.794101, "Pic", "Car"},
	{106.689786, 7.760576, "Mon", "CMi"},
	{106.692780, 7.838962, "Mon", "CMi"},
	{106.873653, 2.061271, "Mon", "CMi"},
	{106.903417, 4.782471, "Mon", "CMi"},
	{115.953710, -64.304068, "Vol", "Car"},
	{122.260542, 62.663140, "Cam", "UMa"},
	{122.786883, -6.188251, "Mon", "Hya"},
	{137.636360, -24.357497, "Pyx", "Hya"},
	{147.100239, 72.926713, "Dra", "UMa"},
	{147.657989, -26.815754, "Ant", "Hya"},
	{175.571414, 65.808649, "Dra", "UMa"},
	{211.613786, 53.404453, "Boo", "UMa"},
	{211.689778, 48.708913, "Boo", "UMa"},
	{214.637628, -54.499803, "Lup", "Cen"},
	{218.510401, -55.554025, "Cen", "Lup"},
	{222.481686, -25.022011, "Hya", "Lib"},
	{224.494271, -30.005078, "Cen", "Hya"},
	{240.412855, -5.813023, "Oph", "Lib"},
	{240.413651, -6.016798, "Oph", "Lib"},
	{243.311682, 39.682703, "Her", "CrB"},
	{245.837039, -20.475507, "Oph", "Sco"},
	{253.202237, -27.789285, "Oph", "Sco"},
	{259.235476, -10.758950, "Ser", "Oph"},
	{272.314821, -45.555546, "Tel", "Ara"},
	{273.404203, -68.847542, "Pav", "Aps"},
	{273.689062, 38.073585, "Lyr", "Her"},
	{284.383473, 18.059272, "Aql", "Her"},
	{284.402769, 16.554025, "Aql", "Her"},
	{287.049887, 50.439166, "Cyg", "Dra"},
	{289.622175, -38.120264, "Sgr", "CrA"},
	{289.731024, -45.277446, "CrA", "Tel"},
	{289.771748, -45.023158, "Sgr", "CrA"},
	{301.756602, -14.082523, "Cap", "Sgr"},
	{306.543960, 67.083839, "Cep", "Dra"},
	{314.352262, -14.520118, "Aqr", "Cap"},
	{327.933309, -74.417298, "Ind", "Oct"},
	{349.249592, 35.181123, "Peg", "And"},
	{351.344498, 53.185378, "And", "Cas"},
	{354.442577, -39.305222, "Scl", "Phe"},
	{357.809417, 48.691346, "And", "Cas"},
}

// TestLookupAgreesWithRomanAtTheBoundaries holds Lookup to Roman's method
// where it is hardest to get right: within about 22" of a boundary, so an
// error the size of annual aberration, or a missing or wrong precession to
// B1875, puts a position in the neighboring constellation.
//
// Over all 200,000 random positions Lookup agreed with Roman's table at
// 199,999 (#570). The exception lies 0.7" from the 15h40m boundary, where
// astrogo's IAU 1976 precession and astropy's FK5 precession disagree, and is
// not one of these points.
func TestLookupAgreesWithRomanAtTheBoundaries(t *testing.T) {
	t.Parallel()

	for _, p := range romanBoundaryPoints {
		_, abbr, err := constellation.Lookup(coord.NewICRS(angle.Deg(p.raDeg), angle.Deg(p.decDeg)))
		if err != nil {
			t.Errorf("Lookup(%.6f, %.6f): %v", p.raDeg, p.decDeg, err)

			continue
		}

		if abbr != p.roman {
			t.Errorf("Lookup(%.6f, %.6f) = %s, Roman's table gives %s (astropy's get_constellation, with aberration, gives %s)",
				p.raDeg, p.decDeg, abbr, p.roman, p.astropy)
		}
	}
}
