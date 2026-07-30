package constants

// SIExactSet holds fundamental constants that became exact by definition
// with the 2019 SI redefinition (the 26th CGPM's 2018 Resolution 1, in
// force since 20 May 2019): the speed of light, the Planck constant, and
// the Boltzmann constant now fix the metre, kilogram, and kelvin (via the
// second and other exact defining constants) rather than being measured
// against them. None of these three has a CODATA vintage the way G, the
// electron/proton mass, and the fine-structure constant do — see CODATA2022.
type SIExactSet struct {
	// Vintage names this realization of the SI.
	Vintage string

	SpeedOfLight      Constant
	PlanckConstant    Constant
	BoltzmannConstant Constant
}

// Name reports the set's vintage, implementing [Set].
func (s SIExactSet) Name() string { return s.Vintage }

// All returns every member of the set, in declaration order, implementing
// [Set].
func (s SIExactSet) All() []Constant {
	return []Constant{s.SpeedOfLight, s.PlanckConstant, s.BoltzmannConstant}
}

// SI2019 is the set of SI defining constants fixed exact by the 2019 SI
// redefinition. Treat it as read-only: it is a package-level var only
// because a Constant (which embeds a unit.Unit) cannot be a Go const.
var SI2019 = SIExactSet{
	Vintage: "SI 2019",

	SpeedOfLight: Constant{
		Name: "speed of light in vacuum", Symbol: "c",
		Value: 299_792_458.0, Unit: meterPerSecond,
		Reference: "BIPM SI Brochure, 9th ed. (2019) — 26th CGPM (2018) Resolution 1",
		Exact:     true,
	},
	// PlanckConstant (h) fixes the kilogram since the 2019 redefinition.
	PlanckConstant: Constant{
		Name: "Planck constant", Symbol: "h",
		Value: 6.626_070_15e-34, Unit: jouleSecond,
		Reference: "BIPM SI Brochure, 9th ed. (2019) — 26th CGPM (2018) Resolution 1",
		Exact:     true,
	},
	// BoltzmannConstant (k_B, not the bare "k" some tables use — kept
	// unambiguous against the many other uses of "k" elsewhere in
	// astronomy) fixes the kelvin since the 2019 redefinition.
	BoltzmannConstant: Constant{
		Name: "Boltzmann constant", Symbol: "k_B",
		Value: 1.380_649e-23, Unit: joulePerKelvin,
		Reference: "BIPM SI Brochure, 9th ed. (2019) — 26th CGPM (2018) Resolution 1",
		Exact:     true,
	},
}
