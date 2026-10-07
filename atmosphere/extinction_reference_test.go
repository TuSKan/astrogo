package atmosphere_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// magPerTau converts an optical depth into magnitudes: 2.5 log10(e).
const magPerTau = 2.5 * math.Log10E

// patatParanal is Patat et al. (2011, A&A 527, A91) Table B.1, the best-fit
// extinction curve for Cerro Paranal from their spectrophotometry of standard
// stars: wavelength in angstroms, k in magnitudes per airmass, and its error.
// The last four rows Patat et al. interpolated from their LBLRTM model,
// without a measurement behind them, to carry the curve past the water bands.
var patatParanal = []struct {
	angstrom, k, sigma float64
	lblrtm             bool
}{
	{3325, 0.686, 0.021, false},
	{3375, 0.606, 0.009, false},
	{3425, 0.581, 0.007, false},
	{3475, 0.552, 0.007, false},
	{3525, 0.526, 0.006, false},
	{3575, 0.504, 0.006, false},
	{3625, 0.478, 0.006, false},
	{3675, 0.456, 0.006, false},
	{3725, 0.430, 0.006, false},
	{3775, 0.409, 0.005, false},
	{3825, 0.386, 0.006, false},
	{3875, 0.378, 0.006, false},
	{3925, 0.363, 0.005, false},
	{3975, 0.345, 0.004, false},
	{4025, 0.330, 0.004, false},
	{4075, 0.316, 0.004, false},
	{4125, 0.298, 0.004, false},
	{4175, 0.285, 0.004, false},
	{4225, 0.274, 0.004, false},
	{4275, 0.265, 0.004, false},
	{4325, 0.253, 0.004, false},
	{4375, 0.241, 0.003, false},
	{4425, 0.229, 0.003, false},
	{4475, 0.221, 0.003, false},
	{4525, 0.212, 0.003, false},
	{4575, 0.204, 0.003, false},
	{4625, 0.198, 0.003, false},
	{4675, 0.190, 0.003, false},
	{4725, 0.185, 0.003, false},
	{4775, 0.182, 0.003, false},
	{4825, 0.176, 0.003, false},
	{4875, 0.169, 0.003, false},
	{4925, 0.162, 0.003, false},
	{4975, 0.157, 0.003, false},
	{5025, 0.156, 0.003, false},
	{5075, 0.153, 0.003, false},
	{5125, 0.146, 0.003, false},
	{5175, 0.143, 0.003, false},
	{5225, 0.141, 0.003, false},
	{5275, 0.139, 0.003, false},
	{5325, 0.139, 0.002, false},
	{5375, 0.134, 0.002, false},
	{5425, 0.133, 0.002, false},
	{5475, 0.131, 0.002, false},
	{5525, 0.129, 0.002, false},
	{5575, 0.127, 0.002, false},
	{5625, 0.128, 0.002, false},
	{5675, 0.130, 0.002, false},
	{5725, 0.134, 0.002, false},
	{5775, 0.132, 0.002, false},
	{5825, 0.124, 0.002, false},
	{5875, 0.122, 0.003, false},
	{5925, 0.125, 0.003, false},
	{5975, 0.122, 0.003, false},
	{6025, 0.117, 0.002, false},
	{6075, 0.115, 0.002, false},
	{6125, 0.108, 0.002, false},
	{6175, 0.104, 0.002, false},
	{6225, 0.102, 0.002, false},
	{6275, 0.099, 0.002, false},
	{6325, 0.095, 0.002, false},
	{6375, 0.092, 0.002, false},
	{6425, 0.085, 0.002, false},
	{6475, 0.086, 0.003, false},
	{6525, 0.083, 0.003, false},
	{6575, 0.081, 0.002, false},
	{6625, 0.076, 0.002, false},
	{6675, 0.072, 0.002, false},
	{6725, 0.068, 0.002, false},
	{6775, 0.064, 0.002, false},
	{7060, 0.064, 0.003, false},
	{7450, 0.048, 0.002, false},
	{7940, 0.042, 0.003, false},
	{8500, 0.032, 0, true},
	{8675, 0.030, 0, true},
	{8850, 0.029, 0, true},
	{10000, 0.022, 0, true},
}

// butonMaunaKea is Buton et al. (2013, A&A 549, A8) Table 6, the median
// extinction above Mauna Kea and its decomposition: wavelength in angstroms,
// then the total, Rayleigh, ozone and aerosol terms in magnitudes per
// airmass, each printed to 0.001. Table 6's total is the sum of the three,
// row by row; the telluric lines are excluded from it.
var butonMaunaKea = []struct{ angstrom, total, rayleigh, ozone, aerosol float64 }{
	{3200, 0.856, 0.606, 0.214, 0.036},
	{3300, 0.588, 0.532, 0.021, 0.035},
	{3400, 0.514, 0.469, 0.012, 0.033},
	{3500, 0.448, 0.415, 0.001, 0.032},
	{3600, 0.400, 0.369, 0.000, 0.031},
	{3700, 0.359, 0.329, 0.000, 0.030},
	{3800, 0.323, 0.294, 0.000, 0.029},
	{3900, 0.292, 0.264, 0.000, 0.028},
	{4000, 0.265, 0.238, 0.000, 0.027},
	{4100, 0.241, 0.215, 0.000, 0.026},
	{4200, 0.220, 0.194, 0.000, 0.026},
	{4300, 0.202, 0.176, 0.001, 0.025},
	{4400, 0.185, 0.160, 0.001, 0.024},
	{4500, 0.171, 0.146, 0.001, 0.023},
	{4600, 0.159, 0.134, 0.003, 0.023},
	{4700, 0.147, 0.122, 0.003, 0.022},
	{4800, 0.139, 0.112, 0.005, 0.022},
	{4900, 0.130, 0.103, 0.006, 0.021},
	{5000, 0.125, 0.095, 0.009, 0.021},
	{5100, 0.119, 0.087, 0.012, 0.020},
	{5200, 0.114, 0.081, 0.013, 0.020},
	{5300, 0.113, 0.075, 0.019, 0.019},
	{5400, 0.109, 0.069, 0.022, 0.019},
	{5500, 0.106, 0.064, 0.024, 0.018},
	{5600, 0.107, 0.060, 0.029, 0.018},
	{5700, 0.108, 0.056, 0.035, 0.017},
	{5800, 0.103, 0.052, 0.034, 0.017},
	{5900, 0.098, 0.048, 0.033, 0.017},
	{6000, 0.098, 0.045, 0.037, 0.016},
	{6100, 0.092, 0.042, 0.034, 0.016},
	{6200, 0.084, 0.039, 0.029, 0.016},
	{6300, 0.078, 0.037, 0.026, 0.015},
	{6400, 0.070, 0.035, 0.021, 0.015},
	{6500, 0.065, 0.033, 0.018, 0.015},
	{6600, 0.060, 0.031, 0.015, 0.014},
	{6700, 0.056, 0.029, 0.013, 0.014},
	{6800, 0.052, 0.027, 0.011, 0.014},
	{6900, 0.048, 0.026, 0.008, 0.014},
	{7000, 0.044, 0.024, 0.007, 0.013},
	{7100, 0.042, 0.023, 0.006, 0.013},
	{7200, 0.039, 0.022, 0.005, 0.013},
	{7300, 0.037, 0.020, 0.004, 0.013},
	{7400, 0.035, 0.019, 0.003, 0.013},
	{7500, 0.033, 0.018, 0.003, 0.012},
	{7600, 0.032, 0.017, 0.002, 0.012},
	{7700, 0.030, 0.016, 0.002, 0.012},
	{7800, 0.029, 0.016, 0.002, 0.012},
	{7900, 0.028, 0.015, 0.002, 0.012},
	{8000, 0.027, 0.014, 0.001, 0.011},
	{8100, 0.026, 0.013, 0.001, 0.011},
	{8200, 0.025, 0.013, 0.001, 0.011},
	{8300, 0.024, 0.012, 0.001, 0.011},
	{8400, 0.023, 0.012, 0.001, 0.011},
	{8500, 0.023, 0.011, 0.001, 0.011},
	{8600, 0.022, 0.011, 0.001, 0.010},
	{8700, 0.021, 0.010, 0.001, 0.010},
	{8800, 0.021, 0.010, 0.001, 0.010},
	{8900, 0.020, 0.009, 0.001, 0.010},
	{9000, 0.019, 0.009, 0.001, 0.010},
	{9100, 0.019, 0.008, 0.001, 0.010},
	{9200, 0.018, 0.008, 0.001, 0.010},
	{9300, 0.018, 0.008, 0.001, 0.009},
	{9400, 0.017, 0.007, 0.001, 0.009},
	{9500, 0.017, 0.007, 0.000, 0.009},
	{9600, 0.016, 0.007, 0.000, 0.009},
	{9700, 0.016, 0.006, 0.000, 0.009},
	{9800, 0.015, 0.006, 0.000, 0.009},
	{9900, 0.015, 0.006, 0.000, 0.009},
	{10000, 0.014, 0.006, 0.000, 0.009},
}

// paperAir is the air a paper reports its extinction under: its surface
// pressure, its ozone column, and its aerosol as both papers give it, a
// magnitude per airmass at 1 µm and an Ångström exponent.
//
// Temperature enters none of the three terms. Single-scattering albedo and
// asymmetry say where scattered light goes, not how much a star loses, so
// they are set to anything valid.
func paperAir(t *testing.T, pressureHPa, ozoneDU, aerosolMagAt1um, angstrom float64) *atmosphere.Atmosphere {
	t.Helper()

	air, err := atmosphere.NewBuilder().
		Surface(pressureHPa, 288.15).
		Ozone(ozoneDU).
		Aerosol(aerosolMagAt1um/magPerTau, 1000, angstrom, 1, 0).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	return air
}

// extinctionAt is Extinction at a wavelength given in angstroms, as both
// papers tabulate it.
func extinctionAt(t *testing.T, air *atmosphere.Atmosphere, angstrom float64) float64 {
	t.Helper()

	k, err := air.Extinction(unit.WavelengthNM(angstrom / 10))
	if err != nil {
		t.Fatalf("Extinction at %g Å: %v", angstrom, err)
	}

	return k
}

// CleanMountainAOD550 is Paranal's measured median aerosol, Patat et al.'s
// 0.013 λ^-1.38 mag per airmass read at 550 nm as an optical depth, to the
// three decimals it is printed to. And it is not the cleanest mountain, as its
// comment says: Buton et al.'s Mauna Kea median lies below it.
func TestCleanMountainAODIsParanalsMedian(t *testing.T) {
	t.Parallel()

	paranal := 0.013 * math.Pow(0.55, -1.38) / magPerTau
	maunaKea := 0.0084 * math.Pow(0.55, -1.26) / magPerTau

	if diff := atmosphere.CleanMountainAOD550 - paranal; math.Abs(diff) > 0.0005 {
		t.Errorf("CleanMountainAOD550 is %g, but Patat et al.'s Paranal median is %.4f at 550 nm",
			atmosphere.CleanMountainAOD550, paranal)
	}

	if maunaKea >= atmosphere.CleanMountainAOD550 {
		t.Errorf("Buton et al.'s Mauna Kea median is %.4f at 550 nm, not below CleanMountainAOD550 %g",
			maunaKea, atmosphere.CleanMountainAOD550)
	}

	t.Logf("550 nm optical depth: Paranal %.4f, Mauna Kea %.4f", paranal, maunaKea)
}

// Extinction reproduces the curve Patat et al. (2011) measured at Cerro
// Paranal, on the air they measured it under.
//
// Their air: 743.6 mb, the campaign's mean pressure; 258 DU, the mean column
// they derive from the Chappuis band; and aerosol k = 0.013 λ^-1.38 with λ in
// µm, their fit to the data.
//
// The bound is 0.01 mag per airmass, the accuracy Patat et al. set out to
// deliver the curve to. Two sets of rows are left out, and logged rather than
// asserted. The four LBLRTM rows, because they are a model and not a
// measurement. And the three rows blueward of 3475 Å: there Patat et al.'s own
// model departs from their data too, by a systematic trend they could not
// explain, and RayleighOpticalDepth is 3 to 4% low (#592).
//
// Measured: within 0.0097 mag per airmass, worst at 3925 Å.
func TestExtinctionReproducesParanal(t *testing.T) {
	t.Parallel()

	const (
		bound    = 0.01 // mag per airmass: Patat et al.'s stated accuracy
		blueEdge = 3475 // Å
	)

	air := paperAir(t, 743.6, 258, 0.013, 1.38)

	var worst, worstAt float64

	for _, row := range patatParanal {
		got := extinctionAt(t, air, row.angstrom)
		diff := got - row.k

		if row.lblrtm || row.angstrom < blueEdge {
			t.Logf("%5.0f Å, not asserted: %.4f against Patat's %.3f (%+.4f)", row.angstrom, got, row.k, diff)
			continue
		}

		if math.Abs(diff) > math.Abs(worst) {
			worst, worstAt = diff, row.angstrom
		}

		if math.Abs(diff) > bound {
			t.Errorf("%5.0f Å: %.4f mag/airmass against Patat's measured %.3f ± %.3f, %+.4f apart; "+
				"the bound is %g", row.angstrom, got, row.k, row.sigma, diff, bound)
		}
	}

	t.Logf("worst from %d Å to 7940 Å: %+.4f mag/airmass at %.0f Å (bound %g)", blueEdge, worst, worstAt, bound)
}

// Extinction's three terms agree with Buton et al.'s (2013) decomposition of
// the median extinction above Mauna Kea, on their median air: 616 mbar, 257.4
// DU, and aerosol 0.0084 (λ/1 µm)^-1.26 mag per airmass.
//
// Table 6 is a decomposition by models at fitted parameters: Hansen & Travis
// (1974) for Rayleigh, a MODTRAN template for ozone, and its total is their
// sum. So this checks each of astrogo's terms against another implementation
// of it, not against a measurement; Paranal, above, is the measurement.
//
// Each bound comes from the paper rather than from this comparison, and
// carries Table 6's printing to 0.001.
//
//   - Rayleigh, from 4000 Å: 1.5%. Buton et al.'s Fig. 2 puts four published
//     formulas within 1.5% of one another there. The power law
//     RayleighOpticalDepth uses leaves that band in the ultraviolet (#592), so
//     the comparison starts where it holds, and the bluer rows are logged.
//     Measured: 1.3% at 4000 Å, 0.0032 mag per airmass.
//   - Ozone, from 3500 Å: 0.002, which is 10 DU at the Chappuis peak. Buton
//     et al. put 20 DU at 4 mmag per airmass there, and the column itself
//     scatters by 23.3 DU from night to night above Mauna Kea. Blueward of
//     3500 Å the Huggins band falls by an order of magnitude in 100 Å and
//     depends on temperature, so a 10 nm average of a 223 K laboratory
//     spectrum and a MODTRAN template part there. Measured: 0.0014.
//   - The total, from 4000 Å: the two above, plus the aerosol printed to
//     0.001, since Table 6's aerosol column is the median of each night's
//     curve rather than the curve of the median parameters. Measured: 0.0035.
func TestExtinctionReproducesMaunaKeasComponents(t *testing.T) {
	t.Parallel()

	const (
		rayleighSpread = 0.015  // Buton et al. Fig. 2, from 4000 to 9000 Å
		printed        = 0.0005 // half of Table 6's last digit
		ozoneBound     = 0.002  // mag per airmass, from 3500 Å: 10 DU at 6000 Å
		aerosolPrinted = 0.001  // mag per airmass
	)

	rayleighBound := func(row float64) float64 { return rayleighSpread*row + printed }

	air := paperAir(t, 616, 257.4, 0.0084, 1.26)
	molecular := paperAir(t, 616, 0, 0, 0)

	type worst struct{ diff, at float64 }

	var wr, wo, wt worst

	track := func(w *worst, diff, at float64) {
		if math.Abs(diff) > math.Abs(w.diff) {
			*w = worst{diff, at}
		}
	}

	for _, row := range butonMaunaKea {
		rayleigh := extinctionAt(t, molecular, row.angstrom)
		total := extinctionAt(t, air, row.angstrom)
		ozone := total - rayleigh - magPerTau*float64(air.Aerosol().TauAt(unit.WavelengthNM(row.angstrom/10)))

		dr, do, dt := rayleigh-row.rayleigh, ozone-row.ozone, total-row.total

		if row.angstrom < 4000 {
			t.Logf("%5.0f Å, Rayleigh not asserted: %.4f against Buton's %.3f (%+.4f)",
				row.angstrom, rayleigh, row.rayleigh, dr)
		} else {
			track(&wr, dr, row.angstrom)
			track(&wt, dt, row.angstrom)

			if bound := rayleighBound(row.rayleigh); math.Abs(dr) > bound {
				t.Errorf("%5.0f Å: Rayleigh %.4f against Buton's %.3f, %+.4f apart; the bound is %.4f",
					row.angstrom, rayleigh, row.rayleigh, dr, bound)
			}

			if bound := rayleighBound(row.rayleigh) + ozoneBound + aerosolPrinted; math.Abs(dt) > bound {
				t.Errorf("%5.0f Å: total %.4f against Buton's %.3f, %+.4f apart; the bound is %.4f",
					row.angstrom, total, row.total, dt, bound)
			}
		}

		if row.angstrom < 3500 {
			t.Logf("%5.0f Å, ozone not asserted: %.4f against Buton's %.3f (%+.4f)",
				row.angstrom, ozone, row.ozone, do)

			continue
		}

		track(&wo, do, row.angstrom)

		if math.Abs(do) > ozoneBound {
			t.Errorf("%5.0f Å: ozone %.4f against Buton's %.3f, %+.4f apart; the bound is %g",
				row.angstrom, ozone, row.ozone, do, ozoneBound)
		}
	}

	t.Logf("worst Rayleigh from 4000 Å: %+.4f at %.0f Å (1.5%% + %g)", wr.diff, wr.at, printed)
	t.Logf("worst ozone from 3500 Å:    %+.4f at %.0f Å (bound %g)", wo.diff, wo.at, ozoneBound)
	t.Logf("worst total from 4000 Å:    %+.4f at %.0f Å", wt.diff, wt.at)
}
