package constants

// PhotometricSet holds fixed, defined photometric reference values — the
// same "single fixed value from a cited authority" shape as IAU2015's
// ObliquityJ2000/SunGravitationalParameter exception (see doc.go's Scope
// section). It deliberately does NOT hold Vega zero points: those depend on
// which Vega reference spectrum is used (Hayes vs. CALSPEC alpha_lyr_stis,
// ...) and are therefore series-valued/model-dependent, out of this
// package's scope — see skybrightness's passband bundle instead.
type PhotometricSet struct {
	// Vintage names the defining publication.
	Vintage string

	ABZeroPoint Constant
}

// Name reports the set's vintage, implementing [Set].
func (s PhotometricSet) Name() string { return s.Vintage }

// All returns every member of the set, in declaration order, implementing
// [Set].
func (s PhotometricSet) All() []Constant {
	return []Constant{s.ABZeroPoint}
}

// Photometric holds fixed, cited photometric reference values. Treat it as
// read-only.
var Photometric = PhotometricSet{
	Vintage: "photometric",

	// ABZeroPoint is the AB magnitude system's defining flux-density zero
	// point: m_AB = -2.5*log10(f_nu / 3631 Jy). Fixed by definition, not
	// measured — Oke & Gunn (1983) defined the AB system this way
	// precisely so it would need no photometric standard star.
	ABZeroPoint: Constant{
		Name: "AB magnitude zero point", Symbol: "f_nu,AB",
		Value: 3631e-26, Unit: wattPerSquareMeterHertz, // 3631 Jy, in SI base (Jy = 1e-26 W/(m2 Hz))
		Reference: "Oke & Gunn (1983), ApJ 266, 713 — AB magnitude system definition",
		Exact:     true,
	},
}
