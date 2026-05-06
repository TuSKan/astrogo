package coord

import "github.com/TuSKan/astrogo/vector"

// ReduceBatch converts a batch of geocentric ICRS position vectors to local
// observed AltAz coordinates using the precomputed Context cache.
// Results are written into the pre-allocated out slice (caller owns memory).
// len(out) must equal len(in); panics otherwise.
//
// The Context's precomputed C2t06a matrix and observer ICRS vector are reused
// for all elements, amortizing the expensive SOFA Apco13 call across the batch.
// For a 10k-star batch, this hits memory bandwidth — not SOFA computation.
//
// This is the natural shape for an Apache Beam ParDo / Go errgroup worker:
//
//	ctx := coord.NewContext(t, site, atm)
//	in := make([]vector.Vec3, len(stars))
//	out := make([]coord.AltAz, len(stars))
//	ctx.ReduceBatch(in, out)
func (ctx *Context) ReduceBatch(in []vector.Vec3, out []AltAz) {
	if len(out) != len(in) {
		panic("coord: ReduceBatch: len(out) must equal len(in)")
	}
	for i, v := range in {
		out[i] = ctx.GeocentricToObserved(v)
	}
}

// ICRSBatchToAltAz converts a batch of ICRS coordinates to AltAz using
// the precomputed ASTROM cache (stellar path: Atciq + Atioq).
// Results are written into the pre-allocated out slice (caller owns memory).
// len(out) must equal len(in); panics otherwise.
//
// This avoids per-call Context creation overhead for stellar targets:
//
//	ctx := coord.NewContext(t, site, atm)
//	stars := []coord.ICRS{...}
//	out := make([]coord.AltAz, len(stars))
//	ctx.ICRSBatchToAltAz(stars, out)
func (ctx *Context) ICRSBatchToAltAz(in []ICRS, out []AltAz) {
	if len(out) != len(in) {
		panic("coord: ICRSBatchToAltAz: len(out) must equal len(in)")
	}
	for i, c := range in {
		// Use the full astrometric pipeline (Atcoq) via the cached ASTROM.
		altaz := ctx.AstrometricToObserved(c.Astrometric())
		altaz.SetDist(c.Dist())
		out[i] = altaz
	}
}
