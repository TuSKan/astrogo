package constants

import "github.com/TuSKan/astrogo/unit"

// CODATASet holds fundamental physical constants that are genuinely
// measured, not fixed by definition — unlike SpeedOfLight/PlanckConstant/
// BoltzmannConstant (see SI2019), the Newtonian gravitational constant,
// electron and proton masses, the fine-structure constant, and the
// Thomson cross section all carry a real, nonzero standard uncertainty
// that shifts (usually only in the last few digits) between CODATA
// adjustments. This is exactly why they are published as separate,
// individually addressable vintages (CODATA2022, CODATA2018) instead of
// one silently-updated symbol: a caller reproducing a published reduction
// can pin the vintage it used.
type CODATASet struct {
	// Vintage names this CODATA adjustment, e.g. "CODATA 2022".
	Vintage string

	GravitationalConstant Constant
	ElectronMass          Constant
	ProtonMass            Constant
	FineStructureConstant Constant
	ThomsonCrossSection   Constant
}

// Name reports the set's vintage, implementing [Set].
func (s CODATASet) Name() string { return s.Vintage }

// All returns every member of the set, in declaration order, implementing
// [Set].
func (s CODATASet) All() []Constant {
	return []Constant{
		s.GravitationalConstant, s.ElectronMass, s.ProtonMass,
		s.FineStructureConstant, s.ThomsonCrossSection,
	}
}

// CODATA2022 is the 2022 CODATA adjustment of the fundamental physical
// constants, verified against physics.nist.gov/cuu/Constants/Table/allascii.txt.
// Treat it as read-only.
//
// GravitationalConstant is bit-identical to CODATA2018's — the 2022
// adjustment did not move G. This is real, not a copy-paste error.
var CODATA2022 = CODATASet{
	Vintage: "CODATA 2022",

	GravitationalConstant: Constant{
		Name: "Newtonian constant of gravitation", Symbol: "G",
		Value: 6.674_30e-11, Uncertainty: 0.000_15e-11,
		Unit:      cubicMeterPerKilogramSecondSquared,
		Reference: "CODATA 2022 (NIST SP 961 / physics.nist.gov/cuu/Constants/)",
	},
	ElectronMass: Constant{
		Name: "electron mass", Symbol: "m_e",
		Value: 9.109_383_7139e-31, Uncertainty: 0.000_000_0028e-31,
		Unit:      unit.Kilogram,
		Reference: "CODATA 2022 (NIST SP 961 / physics.nist.gov/cuu/Constants/)",
	},
	ProtonMass: Constant{
		Name: "proton mass", Symbol: "m_p",
		Value: 1.672_621_925_95e-27, Uncertainty: 0.000_000_000_52e-27,
		Unit:      unit.Kilogram,
		Reference: "CODATA 2022 (NIST SP 961 / physics.nist.gov/cuu/Constants/)",
	},
	FineStructureConstant: Constant{
		Name: "fine-structure constant", Symbol: "α",
		Value: 7.297_352_5643e-3, Uncertainty: 0.000_000_0011e-3,
		Unit:      unit.One,
		Reference: "CODATA 2022 (NIST SP 961 / physics.nist.gov/cuu/Constants/)",
	},
	ThomsonCrossSection: Constant{
		Name: "Thomson cross section", Symbol: "σ_e",
		Value: 6.652_458_7051e-29, Uncertainty: 0.000_000_0062e-29,
		Unit:      squareMeter,
		Reference: "CODATA 2022 (NIST SP 961 / physics.nist.gov/cuu/Constants/)",
	},
}

// CODATA2018 is the 2018 CODATA adjustment, verified against NIST's 2018
// archive table (physics.nist.gov/cuu/Constants/ArchiveASCII/allascii_2018.txt).
// Kept for reproducing a reduction made against this vintage. Treat it as
// read-only.
var CODATA2018 = CODATASet{
	Vintage: "CODATA 2018",

	GravitationalConstant: Constant{
		Name: "Newtonian constant of gravitation", Symbol: "G",
		Value: 6.674_30e-11, Uncertainty: 0.000_15e-11,
		Unit:      cubicMeterPerKilogramSecondSquared,
		Reference: "CODATA 2018 (Tiesinga et al. 2021, J. Phys. Chem. Ref. Data 50, 033105)",
	},
	ElectronMass: Constant{
		Name: "electron mass", Symbol: "m_e",
		Value: 9.109_383_7015e-31, Uncertainty: 0.000_000_0028e-31,
		Unit:      unit.Kilogram,
		Reference: "CODATA 2018 (Tiesinga et al. 2021, J. Phys. Chem. Ref. Data 50, 033105)",
	},
	ProtonMass: Constant{
		Name: "proton mass", Symbol: "m_p",
		Value: 1.672_621_923_69e-27, Uncertainty: 0.000_000_000_51e-27,
		Unit:      unit.Kilogram,
		Reference: "CODATA 2018 (Tiesinga et al. 2021, J. Phys. Chem. Ref. Data 50, 033105)",
	},
	FineStructureConstant: Constant{
		Name: "fine-structure constant", Symbol: "α",
		Value: 7.297_352_5693e-3, Uncertainty: 0.000_000_0011e-3,
		Unit:      unit.One,
		Reference: "CODATA 2018 (Tiesinga et al. 2021, J. Phys. Chem. Ref. Data 50, 033105)",
	},
	ThomsonCrossSection: Constant{
		Name: "Thomson cross section", Symbol: "σ_e",
		Value: 6.652_458_7321e-29, Uncertainty: 0.000_000_0060e-29,
		Unit:      squareMeter,
		Reference: "CODATA 2018 (Tiesinga et al. 2021, J. Phys. Chem. Ref. Data 50, 033105)",
	},
}

// CODATA is the currently-recommended CODATA adjustment — reference this,
// not CODATA2022 directly, unless you specifically need to pin that exact
// vintage for reproducibility. When a new CODATA adjustment ships, a new
// CODATA20xx var is added and this single assignment is updated to point
// at it — no other call site in this repo needs to change. This is a
// value copied at package-init time, not a live pointer: flipping the
// default happens by editing source and shipping a new astrogo release,
// matching how rarely CODATA adjustments actually occur (roughly every 4
// years), not by runtime mutation of CODATA2022 itself.
var CODATA = CODATA2022
