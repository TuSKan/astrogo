package time

import (
	"math"
	"slices"
)

// tdbMinusTT returns TDB−TT in seconds at the geocenter, for a TT two-part
// Julian Date: the leading terms of Fairhead & Bretagnon (1990), as they
// stand in SOFA's iauDtdb, which evaluates all 787 of them.
//
// Against iauDtdb it is within 0.8 µs over 1900–2100, 0.92 µs over 1600–2400
// and 2.7 µs over −1000 to 3000 (TestTDBMinusTTAgainstDtdb), for 37 sines
// where iauDtdb takes 787: some twenty times faster, which matters because
// every ephemeris lookup converts to TDB, and a microsecond is 30 mm of the
// Earth's orbit. iauDtdb itself is good to 3 ns over 1950–2050 against a DE405
// integration. The date is formally TDB; TT serves "with no practical effect
// on the accuracy" (iauDtdb, note 1).
//
// Until #423 this was five terms, of which only the principal one,
// 0.001657 sin M, matched FB90: the others had arguments of their own, and
// together they were further from iauDtdb than that one term alone — 54 µs
// at worst over 1900–2100, against a documented ±1 µs.
func tdbMinusTT(jdTT1, jdTT2 float64) float64 {
	// Julian millennia from J2000.0, the series' own time argument.
	t := ((jdTT1 - 2451545.0) + jdTT2) / 365250.0

	var w [4]float64

	for k, series := range [...][][3]float64{fb90T0[:], fb90T1[:], fb90T2[:], fb90T3[:]} {
		// Smallest first, as iauDtdb sums them.
		for _, term := range slices.Backward(series) {
			w[k] += term[0] * math.Sin(term[1]*t+term[2])
		}
	}

	return w[0] + t*(w[1]+t*(w[2]+t*w[3]))
}

// fb90T0 to fb90T3 are the leading rows of iauDtdb's table for the series in
// t⁰ to t³: amplitude (s), frequency (rad per Julian millennium) and phase
// (rad), copied as they stand from gofa's port, rows 1–30, 475–477, 680–682
// and 765.
var (
	fb90T0 = [30][3]float64{
		{1656.674564e-6, 6283.075849991, 6.240054195},
		{22.417471e-6, 5753.384884897, 4.296977442},
		{13.839792e-6, 12566.151699983, 6.196904410},
		{4.770086e-6, 529.690965095, 0.444401603},
		{4.676740e-6, 6069.776754553, 4.021195093},
		{2.256707e-6, 213.299095438, 5.543113262},
		{1.694205e-6, -3.523118349, 5.025132748},
		{1.554905e-6, 77713.771467920, 5.198467090},
		{1.276839e-6, 7860.419392439, 5.988822341},
		{1.193379e-6, 5223.693919802, 3.649823730},
		{1.115322e-6, 3930.209696220, 1.422745069},
		{0.794185e-6, 11506.769769794, 2.322313077},
		{0.447061e-6, 26.298319800, 3.615796498},
		{0.435206e-6, -398.149003408, 4.349338347},
		{0.600309e-6, 1577.343542448, 2.678271909},
		{0.496817e-6, 6208.294251424, 5.696701824},
		{0.486306e-6, 5884.926846583, 0.520007179},
		{0.432392e-6, 74.781598567, 2.435898309},
		{0.468597e-6, 6244.942814354, 5.866398759},
		{0.375510e-6, 5507.553238667, 4.103476804},
		{0.243085e-6, -775.522611324, 3.651837925},
		{0.173435e-6, 18849.227549974, 6.153743485},
		{0.230685e-6, 5856.477659115, 4.773852582},
		{0.203747e-6, 12036.460734888, 4.333987818},
		{0.143935e-6, -796.298006816, 5.957517795},
		{0.159080e-6, 10977.078804699, 1.890075226},
		{0.119979e-6, 38.133035638, 4.551585768},
		{0.118971e-6, 5486.777843175, 1.914547226},
		{0.116120e-6, 1059.381930189, 0.873504123},
		{0.137927e-6, 11790.629088659, 1.135934669},
	}
	fb90T1 = [3][3]float64{
		{102.156724e-6, 6283.075849991, 4.249032005},
		{1.706807e-6, 12566.151699983, 4.205904248},
		{0.269668e-6, 213.299095438, 3.400290479},
	}
	fb90T2 = [3][3]float64{
		{4.322990e-6, 6283.075849991, 2.642893748},
		{0.406495e-6, 0.000000000, 4.712388980},
		{0.122605e-6, 12566.151699983, 2.438140634},
	}
	fb90T3 = [1][3]float64{
		{0.143388e-6, 6283.075849991, 1.131453581},
	}
)
