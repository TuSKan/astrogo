package constants_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
)

// TestCODATA_Values2022 re-transcribes the published values independently
// (not by referencing the same literal the package itself uses), so a
// transcription error in the source would actually be caught.
func TestCODATA_Values2022(t *testing.T) {
	s := constants.CODATA2022

	testutil.AssertExact(t, "G value", s.GravitationalConstant.Value, 6.674_30e-11)
	testutil.AssertExact(t, "G uncertainty", s.GravitationalConstant.Uncertainty, 0.000_15e-11)
	testutil.AssertExact(t, "m_e value", s.ElectronMass.Value, 9.109_383_7139e-31)
	testutil.AssertExact(t, "m_e uncertainty", s.ElectronMass.Uncertainty, 0.000_000_0028e-31)
	testutil.AssertExact(t, "m_p value", s.ProtonMass.Value, 1.672_621_925_95e-27)
	testutil.AssertExact(t, "m_p uncertainty", s.ProtonMass.Uncertainty, 0.000_000_000_52e-27)
	testutil.AssertExact(t, "alpha value", s.FineStructureConstant.Value, 7.297_352_5643e-3)
	testutil.AssertExact(t, "alpha uncertainty", s.FineStructureConstant.Uncertainty, 0.000_000_0011e-3)
	testutil.AssertExact(t, "sigma_e value", s.ThomsonCrossSection.Value, 6.652_458_7051e-29)
	testutil.AssertExact(t, "sigma_e uncertainty", s.ThomsonCrossSection.Uncertainty, 0.000_000_0062e-29)
}

func TestCODATA_Values2018(t *testing.T) {
	s := constants.CODATA2018

	testutil.AssertExact(t, "G value", s.GravitationalConstant.Value, 6.674_30e-11)
	testutil.AssertExact(t, "G uncertainty", s.GravitationalConstant.Uncertainty, 0.000_15e-11)
	testutil.AssertExact(t, "m_e value", s.ElectronMass.Value, 9.109_383_7015e-31)
	testutil.AssertExact(t, "m_e uncertainty", s.ElectronMass.Uncertainty, 0.000_000_0028e-31)
	testutil.AssertExact(t, "m_p value", s.ProtonMass.Value, 1.672_621_923_69e-27)
	testutil.AssertExact(t, "m_p uncertainty", s.ProtonMass.Uncertainty, 0.000_000_000_51e-27)
	testutil.AssertExact(t, "alpha value", s.FineStructureConstant.Value, 7.297_352_5693e-3)
	testutil.AssertExact(t, "alpha uncertainty", s.FineStructureConstant.Uncertainty, 0.000_000_0011e-3)
	testutil.AssertExact(t, "sigma_e value", s.ThomsonCrossSection.Value, 6.652_458_7321e-29)
	testutil.AssertExact(t, "sigma_e uncertainty", s.ThomsonCrossSection.Uncertainty, 0.000_000_0060e-29)
}

func TestCODATA_NoneExact(t *testing.T) {
	for _, vintage := range []constants.CODATASet{constants.CODATA2022, constants.CODATA2018} {
		for _, c := range vintage.All() {
			if c.Exact {
				t.Errorf("%s (%s): Exact = true, want false (measured)", vintage.Name(), c.Symbol)
			}

			if c.Uncertainty <= 0 {
				t.Errorf("%s (%s): Uncertainty = %v, want > 0", vintage.Name(), c.Symbol, c.Uncertainty)
			}
		}
	}
}

func TestCODATA_Dimensions(t *testing.T) {
	s := constants.CODATA2022

	if !s.ElectronMass.Unit.Compatible(unit.Kilogram) {
		t.Errorf("ElectronMass.Unit not compatible with kg")
	}

	if !s.ThomsonCrossSection.Unit.Compatible(unit.Meter.PowInt(2)) {
		t.Errorf("ThomsonCrossSection.Unit not compatible with m^2")
	}

	if s.FineStructureConstant.Unit.Dimension != unit.Dimensionless {
		t.Errorf("FineStructureConstant.Unit.Dimension = %+v, want Dimensionless", s.FineStructureConstant.Unit.Dimension)
	}

	wantG := unit.Volume.Div(unit.Mass).Div(unit.Time.PowInt(2))
	if got := s.GravitationalConstant.Unit.Dimension; got != wantG {
		t.Errorf("GravitationalConstant.Unit.Dimension = %+v, want %+v", got, wantG)
	}
}

// TestCODATA_VintagesAgree confirms the 2018 and 2022 adjustments
// describe the same physical quantity: agreement within a k=5 combined
// standard uncertainty. k=2/k=3 is deliberately too tight — the real
// 2018->2022 shifts in alpha, m_e, m_p are ~2.2 sigma and a tighter gate
// would be a false failure on real, published data.
func TestCODATA_VintagesAgree(t *testing.T) {
	pairs := []struct {
		name       string
		v2022, v18 constants.Constant
	}{
		{"G", constants.CODATA2022.GravitationalConstant, constants.CODATA2018.GravitationalConstant},
		{"m_e", constants.CODATA2022.ElectronMass, constants.CODATA2018.ElectronMass},
		{"m_p", constants.CODATA2022.ProtonMass, constants.CODATA2018.ProtonMass},
		{"alpha", constants.CODATA2022.FineStructureConstant, constants.CODATA2018.FineStructureConstant},
		{"sigma_e", constants.CODATA2022.ThomsonCrossSection, constants.CODATA2018.ThomsonCrossSection},
	}

	for _, p := range pairs {
		diff := math.Abs(p.v2022.Value - p.v18.Value)
		combined := 5 * (p.v2022.Uncertainty + p.v18.Uncertainty)

		if combined > 0 && diff > combined {
			t.Errorf("%s: |2022-2018| = %.3g exceeds 5x combined uncertainty %.3g", p.name, diff, combined)
		}

		relDiff := diff / math.Abs(p.v2022.Value)
		if relDiff >= 1e-8 {
			t.Errorf("%s: relative difference between vintages %.3g, want < 1e-8", p.name, relDiff)
		}
	}
}

// TestCODATA_GravitationalConstantUnchanged documents that G really is
// bit-identical between the 2018 and 2022 adjustments — the 2022
// adjustment did not move it. This is real, not a copy-paste bug.
func TestCODATA_GravitationalConstantUnchanged(t *testing.T) {
	if constants.CODATA2022.GravitationalConstant.Value != constants.CODATA2018.GravitationalConstant.Value {
		t.Errorf("G value changed between vintages — if this is a real CODATA update, update this test's expectation")
	}

	if constants.CODATA2022.GravitationalConstant.Uncertainty != constants.CODATA2018.GravitationalConstant.Uncertainty {
		t.Errorf("G uncertainty changed between vintages — if this is a real CODATA update, update this test's expectation")
	}
}

func TestCODATA_ProtonElectronMassRatio(t *testing.T) {
	for _, vintage := range []constants.CODATASet{constants.CODATA2022, constants.CODATA2018} {
		ratio := vintage.ProtonMass.Value / vintage.ElectronMass.Value
		testutil.AssertRelNear(t, vintage.Name()+" m_p/m_e", ratio, 1836.152673, 1e-6)
	}
}

func TestCODATA_InverseFineStructure(t *testing.T) {
	for _, vintage := range []constants.CODATASet{constants.CODATA2022, constants.CODATA2018} {
		inv := 1 / vintage.FineStructureConstant.Value
		testutil.AssertRelNear(t, vintage.Name()+" 1/alpha", inv, 137.035999, 1e-6)
	}
}

func TestCODATA_Names(t *testing.T) {
	if got := constants.CODATA2022.Name(); got != "CODATA 2022" {
		t.Errorf("CODATA2022.Name() = %q, want %q", got, "CODATA 2022")
	}

	if got := constants.CODATA2018.Name(); got != "CODATA 2018" {
		t.Errorf("CODATA2018.Name() = %q, want %q", got, "CODATA 2018")
	}
}

func TestCODATA_CurrentAliasPointsAtLatest(t *testing.T) {
	if constants.CODATA != constants.CODATA2022 {
		t.Errorf("constants.CODATA does not equal constants.CODATA2022 — the current-vintage alias is stale")
	}
}
