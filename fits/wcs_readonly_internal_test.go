package fits

import (
	"reflect"
	"testing"
)

// sipTAN is a TAN-SIP WCS with every part a transform reads set.
func sipTAN(t *testing.T) *WCS {
	t.Helper()

	w := NewWCS(2)

	for _, err := range []error{
		w.SetCTYPE([]string{"RA---TAN-SIP", "DEC--TAN-SIP"}),
		w.SetCRVAL([]float64{150, 2}),
		w.SetCRPIX([]float64{1024, 1024}),
		w.SetCDELT([]float64{-2.8e-4, 2.8e-4}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	w.SetSIP(map[[2]int]float64{{2, 0}: 1.2e-5}, map[[2]int]float64{{1, 1}: 5.6e-6})
	w.SetSIPInverse(map[[2]int]float64{{2, 0}: -1.2e-5}, map[[2]int]float64{{1, 1}: -5.6e-6})

	return w
}

// TestTransformsDoNotWriteTheWCS: PixelToWorld and WorldToPixel leave a WCS
// exactly as they found it. Each used to re-derive the celestial axes from
// CTYPE and write them into the WCS, a data race between two goroutines
// sharing one (#797); a WCS fresh from SetCTYPE still held NewWCS's zero axes
// until its first transform rewrote them.
func TestTransformsDoNotWriteTheWCS(t *testing.T) {
	t.Parallel()

	w := sipTAN(t)
	before := *w

	if _, err := w.PixelToWorld([]float64{100, 200}); err != nil {
		t.Fatalf("PixelToWorld: %v", err)
	}

	if _, err := w.WorldToPixel([]float64{150.2, 1.9}); err != nil {
		t.Fatalf("WorldToPixel: %v", err)
	}

	if !reflect.DeepEqual(before, *w) {
		t.Errorf("the transforms changed the WCS:\nbefore %+v\nafter  %+v", before, *w)
	}
}

// TestSetCTYPEDerivesTheProjection: the projection and the celestial axes are
// computed when CTYPE is set (#797), so setting it again must recompute them,
// in either axis order and back to none.
func TestSetCTYPEDerivesTheProjection(t *testing.T) {
	t.Parallel()

	w := NewWCS(2)

	for _, c := range []struct {
		ctype    []string
		proj     string
		lon, lat int
	}{
		{[]string{"", ""}, "", -1, -1},
		{[]string{"RA---TAN-SIP", "DEC--TAN-SIP"}, "TAN", 0, 1},
		{[]string{"DEC--SIN", "RA---SIN"}, "SIN", 1, 0},
		{[]string{"GLON-AIT", "GLAT-AIT"}, "AIT", 0, 1},
		{[]string{"LINEAR", "LINEAR"}, "", -1, -1},
	} {
		if err := w.SetCTYPE(c.ctype); err != nil {
			t.Fatalf("SetCTYPE(%q): %v", c.ctype, err)
		}

		if w.proj != c.proj || w.lonAxis != c.lon || w.latAxis != c.lat {
			t.Errorf("SetCTYPE(%q): projection %q, axes %d, %d; want %q, %d, %d",
				c.ctype, w.proj, w.lonAxis, w.latAxis, c.proj, c.lon, c.lat)
		}
	}

	if one := NewWCS(1); one.proj != "" || one.lonAxis != -1 || one.latAxis != -1 {
		t.Errorf("NewWCS(1): projection %q, axes %d, %d; want none", one.proj, one.lonAxis, one.latAxis)
	}
}
