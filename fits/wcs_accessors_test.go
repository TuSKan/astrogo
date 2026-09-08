package fits_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// A WCS says where an image is on the sky, so anything that can change one
// without going through a setter can move a whole image silently. Three ways
// that used to be possible, each measured before it was closed.

// TestGettersReturnACopy: the getters used to hand back the internal slice, so
// a caller reading CRVAL to inspect it could rewrite the reference position by
// accident — and would then be transforming coordinates against a WCS nobody
// meant to change.
func TestGettersReturnACopy(t *testing.T) {
	w := fits.NewWCS(2)

	mustSet(t,
		w.SetCRPIX([]float64{50, 50}),
		w.SetCRVAL([]float64{10, 45}),
		w.SetCDELT([]float64{-0.01, 0.01}),
		w.SetCTYPE([]string{"RA---TAN", "DEC--TAN"}),
	)

	w.CRPIX()[0] = 999
	w.CRVAL()[0] = 999
	w.CDELT()[0] = 999
	w.CTYPE()[0] = "GLON-AIT"
	w.PC()[0][0] = 999

	switch {
	case w.CRPIX()[0] != 50:
		t.Errorf("CRPIX was rewritten through its getter: %v", w.CRPIX())
	case w.CRVAL()[0] != 10:
		t.Errorf("CRVAL was rewritten through its getter: %v", w.CRVAL())
	case w.CDELT()[0] != -0.01:
		t.Errorf("CDELT was rewritten through its getter: %v", w.CDELT())
	case w.CTYPE()[0] != "RA---TAN":
		t.Errorf("CTYPE was rewritten through its getter: %v", w.CTYPE())
	case w.PC()[0][0] != 1:
		t.Errorf("PC was rewritten through its getter: %v", w.PC())
	}
}

// TestSettersCopyTheirInput is the same hole from the other side: a caller who
// kept the slice they passed in could change the WCS afterwards, at a distance,
// with nothing at the call site to suggest it.
func TestSettersCopyTheirInput(t *testing.T) {
	w := fits.NewWCS(2)

	crpix := []float64{50, 50}
	crval := []float64{10, 45}
	cdelt := []float64{-0.01, 0.01}
	ctype := []string{"RA---TAN", "DEC--TAN"}
	pc := [][]float64{{1, 0}, {0, 1}}

	mustSet(t,
		w.SetCRPIX(crpix),
		w.SetCRVAL(crval),
		w.SetCDELT(cdelt),
		w.SetCTYPE(ctype),
		w.SetPC(pc),
	)

	crpix[0], crval[0], cdelt[0] = 999, 999, 999
	ctype[0] = "GLON-AIT"
	pc[0][0] = 999

	switch {
	case w.CRPIX()[0] != 50:
		t.Errorf("CRPIX followed the caller's slice: %v", w.CRPIX())
	case w.CRVAL()[0] != 10:
		t.Errorf("CRVAL followed the caller's slice: %v", w.CRVAL())
	case w.CDELT()[0] != -0.01:
		t.Errorf("CDELT followed the caller's slice: %v", w.CDELT())
	case w.CTYPE()[0] != "RA---TAN":
		t.Errorf("CTYPE followed the caller's slice: %v", w.CTYPE())
	case w.PC()[0][0] != 1:
		t.Errorf("PC followed the caller's row: %v", w.PC())
	}
}

// TestSettersRefuseTheWrongLength: a short array is a panic reachable from
// PixelToWorld, which indexes crpix and crval from 0 to NAxis; a long one is a
// silent disagreement about how many axes this WCS has. Both used to be
// accepted without a word.
func TestSettersRefuseTheWrongLength(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*fits.WCS) error
	}{
		{"CRPIX too short", func(w *fits.WCS) error { return w.SetCRPIX([]float64{1}) }},
		{"CRPIX too long", func(w *fits.WCS) error { return w.SetCRPIX([]float64{1, 2, 3}) }},
		{"CRVAL too short", func(w *fits.WCS) error { return w.SetCRVAL([]float64{1}) }},
		{"CDELT too long", func(w *fits.WCS) error { return w.SetCDELT([]float64{1, 2, 3}) }},
		{"CTYPE too short", func(w *fits.WCS) error { return w.SetCTYPE([]string{"RA---TAN"}) }},
		{"PC wrong row count", func(w *fits.WCS) error { return w.SetPC([][]float64{{1, 0}}) }},
		{"PC ragged row", func(w *fits.WCS) error { return w.SetPC([][]float64{{1, 0}, {0}}) }},
		{"nil", func(w *fits.WCS) error { return w.SetCRVAL(nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := fits.NewWCS(2)

			if err := tc.set(w); !errors.Is(err, fits.ErrWCSDimension) {
				t.Fatalf("err = %v, want ErrWCSDimension", err)
			}
		})
	}
}

// TestARefusedSetLeavesTheWCSAlone: a rejected setter must not half-apply. A PC
// matrix with a good first row and a ragged second one is the case that could —
// the rows are copied one at a time.
func TestARefusedSetLeavesTheWCSAlone(t *testing.T) {
	w := fits.NewWCS(2)

	mustSet(t, w.SetPC([][]float64{{2, 0}, {0, 2}}))

	if err := w.SetPC([][]float64{{9, 9}, {9}}); !errors.Is(err, fits.ErrWCSDimension) {
		t.Fatalf("err = %v, want ErrWCSDimension", err)
	}

	if got := w.PC(); got[0][0] != 2 || got[1][1] != 2 {
		t.Errorf("a refused SetPC left the matrix as %v; it should be unchanged", got)
	}
}

// TestDistortionSettersCopyTheirMaps: SIP coefficients have no length
// invariant, so those setters take no error — but they are still maps the
// caller keeps a reference to, and a distortion that changes under a WCS moves
// every pixel it transforms.
//
// Observed through the transform, since the coefficients have no getter. The
// two WCSes are built identically and only one has its caller-side map
// rewritten afterwards, so any difference is the aliasing and nothing else.
// (My first version of this also set TPV on one side and not the other, and
// the 0.3-degree discrepancy it reported was that, not the property under
// test.)
func TestDistortionSettersCopyTheirMaps(t *testing.T) {
	build := func(a, b map[[2]int]float64) *fits.WCS {
		t.Helper()

		w := fits.NewWCS(2)

		mustSet(t,
			w.SetCTYPE([]string{"RA---TAN", "DEC--TAN"}),
			w.SetCRPIX([]float64{50, 50}),
			w.SetCRVAL([]float64{10, 45}),
			w.SetCDELT([]float64{-0.01, 0.01}),
		)
		w.SetSIP(a, b)

		return w
	}

	reference := build(
		map[[2]int]float64{{0, 2}: 1e-6},
		map[[2]int]float64{{2, 0}: 2e-6},
	)

	a := map[[2]int]float64{{0, 2}: 1e-6}
	b := map[[2]int]float64{{2, 0}: 2e-6}
	subject := build(a, b)

	// The caller changes their own map after handing it over.
	a[[2]int{0, 2}] = 1
	b[[2]int{2, 0}] = 1

	pixel := []float64{60, 70}

	want, err := reference.PixelToWorld(pixel)
	if err != nil {
		t.Fatalf("reference PixelToWorld: %v", err)
	}

	got, err := subject.PixelToWorld(pixel)
	if err != nil {
		t.Fatalf("PixelToWorld: %v", err)
	}

	if got[0] != want[0] || got[1] != want[1] {
		t.Errorf("the SIP coefficients followed the caller's map: got %v, want %v", got, want)
	}
}

// TestNilDistortionStaysNil: nil means "no distortion" and an empty map would
// evaluate to the same thing, but only nil says it was never set — and the
// transform's fast path checks length, so a copy that turned nil into an empty
// map would be correct and slower.
func TestNilDistortionStaysNil(t *testing.T) {
	w := fits.NewWCS(2)

	mustSet(t,
		w.SetCTYPE([]string{"RA---TAN", "DEC--TAN"}),
		w.SetCRPIX([]float64{50, 50}),
		w.SetCRVAL([]float64{10, 45}),
		w.SetCDELT([]float64{-0.01, 0.01}),
	)

	w.SetSIP(nil, nil)
	w.SetSIPInverse(nil, nil)
	w.SetTPV(nil, nil)

	// Nothing to assert but that it still transforms: a nil map turned into a
	// non-nil empty one would take the distortion path and still be right, so
	// this is a smoke test rather than a proof.
	if _, err := w.PixelToWorld([]float64{50, 50}); err != nil {
		t.Errorf("a WCS with nil distortion maps failed to transform: %v", err)
	}
}

// TestNAxisReportsTheInvariant: every setter's length check is against this, so
// a caller building a WCS from a header needs to be able to ask.
func TestNAxisReportsTheInvariant(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4} {
		if got := fits.NewWCS(n).NAxis(); got != n {
			t.Errorf("NewWCS(%d).NAxis() = %d", n, got)
		}
	}
}
