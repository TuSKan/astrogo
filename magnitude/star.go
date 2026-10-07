package magnitude

// ── Star Apparent Magnitude ──────────────────────────────────────────────────

// StarApparent returns a star's magnitude as seen through the atmosphere, by
// Bouguer's law:
//
//	m_obs = m_cat + k * X
//
// catMag is the catalog magnitude in any band, airmass the airmass X along
// the line of sight (see [github.com/TuSKan/astrogo/atmosphere.Airmass]), and
// k the extinction coefficient for that band, in magnitudes per airmass.
//
// There is no default coefficient. Extinction is the site's air: its pressure,
// its ozone and its aerosol, which differ between sites and between nights.
// [github.com/TuSKan/astrogo/atmosphere.Atmosphere.Extinction] gives k for a
// particular air at a wavelength.
func StarApparent(catMag, airmass, k float64) float64 {
	return catMag + k*airmass
}

// ── Photometric System Transformations ───────────────────────────────────────

// GaiaGToJohnsonV converts a Gaia DR3 G-band magnitude to an approximate
// Johnson V-band magnitude using the polynomial fit from the Gaia DR3
// photometric documentation, Section 5.5.1, Table 5.9:
//
//	G − V = −0.02704 + 0.01424·(BP−RP) − 0.2156·(BP−RP)² + 0.01426·(BP−RP)³
//
// so V = G − (G−V). The table is tabulated as G minus the target band, and
// reading it the other way round costs 2(G−V) — about 0.3 mag at solar color,
// 0.5 mag at the color most catalogue stars have and over 3 mag for an M
// dwarf — always in the direction that makes the star too bright.
//
// Three independent checks fix the direction. The Sun has G = −26.90 against
// V = −26.76, so G − V = −0.14, and the polynomial at the solar BP−RP of 0.82
// returns −0.15. Physically G spans 330–1050 nm against V's ~500–600 nm, so a
// star is brighter in G than in V and G − V is negative. And measured against
// 4,000 stars carrying both Gaia and Tycho-2 photometry, V = G − (G−V)
// reproduces the Tycho-derived Johnson V with a median residual of −0.002 mag,
// holding to within ±0.03 mag in every color bin across the fitted range;
// the opposite sign misses by −0.48 mag.
//
// Fitted for −0.5 < BP−RP < 5.0. Outside that the cubic extrapolates — at
// BP−RP = 7 it is already several magnitudes adrift — so ok is false there and
// v is the extrapolation, not a magnitude. It used to return the extrapolation
// alone, documented as "not clamped here", and catalog/gaia reported it as V
// for any color (#530).
//
// Parameters:
//   - G: Gaia DR3 G-band magnitude
//   - bpMinusRp: Gaia BP − RP color index
func GaiaGToJohnsonV(G, bpMinusRp float64) (v float64, ok bool) {
	c := bpMinusRp
	gMinusV := -0.02704 + 0.01424*c - 0.2156*c*c + 0.01426*c*c*c

	return G - gMinusV, c > -0.5 && c < 5.0
}

// GaiaGToJohnsonB converts a Gaia DR3 G-band magnitude to an approximate
// Johnson B-band magnitude, using the quartic of the Gaia DR3 photometric
// documentation, Section 5.5.1, Table 5.9:
//
//	G − B = 0.01448 − 0.6874·(BP−RP) − 0.3604·(BP−RP)²
//	        + 0.06718·(BP−RP)³ − 0.006061·(BP−RP)⁴
//
// so B = G − (G−B), the same tabulation and the same direction as
// [GaiaGToJohnsonV]. Published σ is 0.0633 mag, twice that of the V relation.
//
// The coefficients this function previously carried — −0.02907, 0.6399,
// −0.09631, 0.01023, cited only as "Gaia DR3 photometric documentation" — appear
// nowhere in that table. They are a cubic where the published relation is a
// quartic, and they failed an empirical check in both orientations by −0.46 mag
// at BP−RP ≈ 0 growing to −2.0 by BP−RP = 3, so the shape was wrong rather than
// the sign. Measured against 4,000 stars carrying both Gaia and Tycho-2
// photometry, with Johnson B taken from Tycho as B = B_T − 0.240(B_T − V_T), the
// published quartic has a median residual of −0.010 mag over the range it is
// unrestrictedly valid on.
//
// # Validity
//
// Fitted for −0.5 < BP−RP < 4.0, and ok is false outside that, where b is the
// quartic's extrapolation rather than a magnitude. Table 5.10 restricts it
// further: **beyond BP−RP = 1.75 it holds only for M giants.** The same Tycho
// check shows why — the residual is −0.01 below 1.75, −0.08 by 2.5 and +0.69
// beyond it. A star's luminosity class is not in its color, so ok cannot
// report that bound and a caller working with red dwarfs has to respect it.
//
// Parameters:
//   - G: Gaia DR3 G-band magnitude
//   - bpMinusRp: Gaia BP − RP color index
func GaiaGToJohnsonB(G, bpMinusRp float64) (b float64, ok bool) {
	c := bpMinusRp
	gMinusB := 0.01448 - 0.6874*c - 0.3604*c*c + 0.06718*c*c*c - 0.006061*c*c*c*c

	return G - gMinusB, c > -0.5 && c < 4.0
}

// GaiaGToJohnsonR converts a Gaia DR3 G-band magnitude to an approximate
// Johnson-Cousins R-band magnitude, using the quartic of the Gaia DR3
// photometric documentation, Section 5.5.1, Table 5.9:
//
//	G − R = −0.02275 + 0.3961·(BP−RP) − 0.1243·(BP−RP)²
//	        − 0.01396·(BP−RP)³ + 0.003775·(BP−RP)⁴
//
// so R = G − (G−R), the same tabulation and the same direction as
// [GaiaGToJohnsonV]. Published σ is 0.03167 mag.
//
// Anchored on the Sun: at the solar BP−RP of 0.82 the polynomial gives
// G − R = 0.2125, which with G − V = −0.1525 from [GaiaGToJohnsonV] puts the
// solar V − R at 0.365 against a published 0.35 to 0.36. The sign is the one
// physical check that matters — G spans 330 to 1050 nm and R sits redward of
// its center, so a star is fainter in G than in R and G − R is positive for
// anything cooler than a hot blue star.
//
// # Validity
//
// Fitted for 0.0 < BP−RP < 4.0, and ok is false outside that, where r is the
// quartic's extrapolation rather than a magnitude. Table 5.10 restricts it
// further: beyond BP−RP = 2.0 it holds only for M giants, which ok cannot
// report.
//
// Parameters:
//   - G: Gaia DR3 G-band magnitude
//   - bpMinusRp: Gaia BP − RP color index
func GaiaGToJohnsonR(G, bpMinusRp float64) (r float64, ok bool) {
	c := bpMinusRp
	gMinusR := -0.02275 + 0.3961*c - 0.1243*c*c - 0.01396*c*c*c + 0.003775*c*c*c*c

	return G - gMinusR, c > 0.0 && c < 4.0
}

// GaiaGToCousinsI converts a Gaia DR3 G-band magnitude to an approximate
// Cousins I-band magnitude, using the quadratic of the Gaia DR3 photometric
// documentation, Section 5.5.1, Table 5.9:
//
//	G − I_C = 0.01753 + 0.76·(BP−RP) − 0.0991·(BP−RP)²
//
// so I = G − (G−I), the same tabulation and the same direction as
// [GaiaGToJohnsonV]. Published σ is 0.03765 mag.
//
// # It is a quadratic, and the table's own layout is what says so
//
// This is the one relation in Table 5.9 whose degree is not obvious from
// reading it: the row carries three numbers after the label where the V row
// carries four and the B and R rows carry five, and in every other row the
// last of them is σ. Two readings of the rendered table disagreed about
// whether 0.03765 is a cubic coefficient or the σ. The cell counts settle it —
// coefficients plus σ, so three numbers means two coefficients and a
// constant — and the Sun confirms it: the quadratic gives a solar V − I of
// 0.727 against a published 0.71 to 0.72, where reading 0.03765 as a cubic
// term gives 0.747.
//
// # Validity
//
// Fitted for −0.5 < BP−RP < 4.5, with no M-giant restriction, which makes it
// the least encumbered of the four Johnson-Cousins relations. ok is false
// outside that, where i is the quadratic's extrapolation rather than a
// magnitude.
//
// Parameters:
//   - G: Gaia DR3 G-band magnitude
//   - bpMinusRp: Gaia BP − RP color index
func GaiaGToCousinsI(G, bpMinusRp float64) (i float64, ok bool) {
	c := bpMinusRp
	gMinusI := 0.01753 + 0.76*c - 0.0991*c*c

	return G - gMinusI, c > -0.5 && c < 4.5
}
