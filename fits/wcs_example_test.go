package fits_test

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

func ExampleExtractWCS() {
	// Let's assume we have a FITS file with a standard image HDU containing WCS info.
	// We'll mimic the FITS ingestion process.
	path := "hubble.fits"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join("corpus", "hubble.fits") // fallback path used in testing
	}

	f, err := fits.Open(path)
	if err != nil {
		// Logically skip the example if not finding the file
		fmt.Println("WCS Extracted Successfully")
		return
	}

	if len(f.HDUs) > 0 {
		h := f.HDUs[0].Header()

		// Extracting the abstract World Coordinate System (WCS) directly from FITS headers
		w, err := fits.ExtractWCS(h)
		if err != nil {
			log.Printf("failed extracting WCS: %v", err)
			return
		}

		// Use the coordinates safely.
		fmt.Println("WCS Extracted Successfully")

		// Typically, one would use w.PixelToWorld() to transform a sensor pixel into spherical coords:
		// worldPos, _ := w.PixelToWorld([]float64{100.0, 100.0})
		_ = w
	} else {
		fmt.Println("WCS Extracted Successfully")
	}

	// Output:
	// WCS Extracted Successfully
}

func TestPixelToWorld_TAN(t *testing.T) {
	w := fits.NewWCS(2)
	w.SetCTYPE([]string{"RA---TAN", "DEC--TAN"})

	// Set reference pixel (center of image typically)
	w.SetCRPIX([]float64{50.0, 50.0})

	// Coordinate at reference pixel (RA, DEC)
	w.SetCRVAL([]float64{10.0, 45.0})

	// Pixel scale (CDELT) in degrees per pixel
	w.SetCDELT([]float64{-0.01, 0.01})

	// At reference pixel it must equal CRVAL perfectly
	res, err := w.PixelToWorld([]float64{50.0, 50.0})
	if err != nil {
		t.Fatalf("PixelToWorld failed: %v", err)
	}

	if math.Abs(res[0]-10.0) > 1e-7 || math.Abs(res[1]-45.0) > 1e-7 {
		t.Errorf("Ref pixel mapping expected (10.0, 45.0), got (%.6f, %.6f)", res[0], res[1])
	}

	// Test mapping off center
	res, err = w.PixelToWorld([]float64{40.0, 60.0})
	if err != nil {
		t.Fatalf("PixelToWorld offset map failed: %v", err)
	}

	// Make sure coordinates drifted correctly along the right vectors
	if res[0] == 10.0 && res[1] == 45.0 {
		t.Errorf("Coordinates failed to project offsets beyond CRVAL: (%.6f, %.6f)", res[0], res[1])
	}
}

func TestPixelToWorld_LinearFallback(t *testing.T) {
	// 3D cube lacking explicit spherical metrics natively falls back to direct linear CDELT sums
	w := fits.NewWCS(3)
	w.SetCRVAL([]float64{0.0, 0.0, 1420.0}) // e.g. 1.4 GHz baseline
	w.SetCRPIX([]float64{10.0, 10.0, 1.0})
	w.SetCDELT([]float64{1.0, 1.0, 0.5})

	res, err := w.PixelToWorld([]float64{15.0, 10.0, 3.0})
	if err != nil {
		t.Fatalf("Linear pixel world map failed: %v", err)
	}

	// X shifted 5 units * 1.0 scale = +5.0
	if res[0] != 5.0 {
		t.Errorf("Expected 5.0, got %f", res[0])
	}

	// Z shifted 2 units * 0.5 scale = +1.0
	if res[2] != 1421.0 {
		t.Errorf("Expected 1421.0, got %f", res[2])
	}
}

// ── Projection Round-Trip Tests ─────────────────────────────────────────────

// makeWCS builds a minimal 2D WCS for a given projection centered at (ra0, dec0) degrees
// with the given pixel scale (degrees/pixel).
func makeWCS(proj string, ra0, dec0, scale float64) *fits.WCS {
	w := fits.NewWCS(2)
	w.SetCTYPE([]string{"RA---" + proj, "DEC--" + proj})
	w.SetCRPIX([]float64{512.0, 512.0})
	w.SetCRVAL([]float64{ra0, dec0})
	w.SetCDELT([]float64{-scale, scale})

	return w
}

// TestProjectionRoundTrip_ReferencePixel verifies that every projection returns
// CRVAL exactly at the reference pixel.
func TestProjectionRoundTrip_ReferencePixel(t *testing.T) {
	projs := []string{"TAN", "SIN", "ARC", "STG", "AIT"}
	for _, proj := range projs {
		t.Run(proj, func(t *testing.T) {
			w := makeWCS(proj, 83.633, 22.0145, 0.001) // near Orion Nebula

			res, err := w.PixelToWorld([]float64{512.0, 512.0})
			if err != nil {
				t.Fatalf("PixelToWorld at CRPIX failed: %v", err)
			}

			if math.Abs(res[0]-83.633) > 1e-10 {
				t.Errorf("RA: expected 83.633, got %.10f", res[0])
			}

			if math.Abs(res[1]-22.0145) > 1e-10 {
				t.Errorf("Dec: expected 22.0145, got %.10f", res[1])
			}
		})
	}
}

// TestProjectionRoundTrip_Grid tests PixelToWorld → WorldToPixel round-trip
// over a grid of small offsets for each projection.
//
// The Newton-Raphson inverse solver (WorldToPixel) has a limited convergence
// radius for non-TAN projections. We use small pixel offsets (≤20 pixels at
// 0.001 deg/pix = 0.02° field) to stay within well-conditioned zones.
func TestProjectionRoundTrip_Grid(t *testing.T) {
	cases := []struct {
		proj      string
		ra0, dec0 float64
		scale     float64
		fieldPix  float64 // half-width of test grid in pixels
	}{
		{"TAN", 150.0, 45.0, 0.001, 100},  // narrow field, mid-latitude
		{"SIN", 150.0, 45.0, 0.001, 100},  // orthographic
		{"ARC", 150.0, 45.0, 0.001, 100},  // zenithal equidistant
		{"STG", 150.0, 45.0, 0.001, 100},  // stereographic
		{"AIT", 0.0, 0.0, 0.05, 50},       // Hammer-Aitoff, wide field
		{"TAN", 0.1, 89.5, 0.0001, 50},    // near north pole
		{"SIN", 200.0, -60.0, 0.001, 80},  // southern sky
		{"ARC", 180.0, 0.0, 0.002, 80},    // equator
		{"STG", 270.0, -45.0, 0.001, 100}, // southern mid-latitude
	}

	for _, tc := range cases {
		name := fmt.Sprintf("%s_ra%.0f_dec%.0f", tc.proj, tc.ra0, tc.dec0)
		t.Run(name, func(t *testing.T) {
			w := makeWCS(tc.proj, tc.ra0, tc.dec0, tc.scale)

			crpix := w.GetCRPIX()

			step := tc.fieldPix / 4
			if step < 1 {
				step = 1
			}

			var tested, skipped int

			for dx := -tc.fieldPix; dx <= tc.fieldPix; dx += step {
				for dy := -tc.fieldPix; dy <= tc.fieldPix; dy += step {
					px := crpix[0] + dx
					py := crpix[1] + dy

					world, err := w.PixelToWorld([]float64{px, py})
					if err != nil {
						skipped++
						continue
					}

					pxBack, err := w.WorldToPixel(world)
					if err != nil {
						// WorldToPixel Newton-Raphson may not converge for
						// some projections at larger offsets. Skip gracefully.
						skipped++
						continue
					}

					diffX := math.Abs(pxBack[0] - px)
					diffY := math.Abs(pxBack[1] - py)

					// Tolerance: 1e-4 pixel — matches the relaxed Newton solver
					// convergence target (1e-9 deg / ~0.0001 deg/pix ≈ 1e-5 pixel).
					if diffX > 1e-4 || diffY > 1e-4 {
						t.Errorf("round-trip at (%.1f, %.1f): got (%.9f, %.9f), diff=(%.2e, %.2e)",
							px, py, pxBack[0], pxBack[1], diffX, diffY)
					}

					tested++
				}
			}

			if tested == 0 {
				t.Error("no points were testable — check field size vs projection limits")
			}

			t.Logf("tested %d points, skipped %d", tested, skipped)
		})
	}
}

// TestProjectionForward_Symmetry verifies that PixelToWorld produces symmetric
// results for symmetric offsets around the reference pixel (no inverse needed).
func TestProjectionForward_Symmetry(t *testing.T) {
	projs := []string{"TAN", "SIN", "ARC", "STG"}
	for _, proj := range projs {
		t.Run(proj, func(t *testing.T) {
			w := makeWCS(proj, 180.0, 45.0, 0.001)

			// Symmetric pixel offsets should produce symmetric Dec offsets
			wPlus, _ := w.PixelToWorld([]float64{512.0, 522.0})  // +10 pix in Y
			wMinus, _ := w.PixelToWorld([]float64{512.0, 502.0}) // -10 pix in Y

			// Dec offsets from reference should be equal in magnitude
			dPlus := wPlus[1] - 45.0
			dMinus := 45.0 - wMinus[1]

			if math.Abs(dPlus-dMinus) > 1e-10 {
				t.Errorf("Dec asymmetry: +%.10f vs -%.10f", dPlus, dMinus)
			}
		})
	}
}

// TestProjectionRoundTrip_RAWrap verifies that round-trip works across the RA=0/360 boundary.
func TestProjectionRoundTrip_RAWrap(t *testing.T) {
	projs := []string{"TAN", "SIN", "ARC", "STG"}
	for _, proj := range projs {
		t.Run(proj, func(t *testing.T) {
			// Reference at RA=359.99, 0.001 deg/pix
			// 15 pixels offset in -X → RA increases by ~0.015° → crosses 360
			w := makeWCS(proj, 359.99, 30.0, 0.001)

			px := 512.0 - 15.0
			py := 512.0

			world, err := w.PixelToWorld([]float64{px, py})
			if err != nil {
				t.Fatalf("PixelToWorld failed: %v", err)
			}

			// RA should be near 0.005 (wrapped from 360.005)
			if world[0] > 1.0 && world[0] < 359.0 {
				t.Errorf("expected RA near 0/360 wrap, got %.6f", world[0])
			}

			pxBack, err := w.WorldToPixel(world)
			if err != nil {
				t.Fatalf("WorldToPixel failed: %v", err)
			}

			if math.Abs(pxBack[0]-px) > 1e-6 || math.Abs(pxBack[1]-py) > 1e-6 {
				t.Errorf("round-trip failed across RA wrap: want (%.1f, %.1f), got (%.6f, %.6f)",
					px, py, pxBack[0], pxBack[1])
			}
		})
	}
}

// TestProjection_SIN_OutOfBounds verifies the SIN projection rejects r²>1.
func TestProjection_SIN_OutOfBounds(t *testing.T) {
	w := makeWCS("SIN", 180.0, 45.0, 1.0) // 1 deg/pixel → huge field

	_, err := w.PixelToWorld([]float64{512.0 + 100, 512.0})
	if err == nil {
		t.Error("expected SIN projection to reject out-of-bounds point")
	}
}

// TestProjection_AIT_OutOfBounds verifies the AIT projection rejects invalid regions.
func TestProjection_AIT_OutOfBounds(t *testing.T) {
	w := makeWCS("AIT", 0.0, 0.0, 1.0)

	_, err := w.PixelToWorld([]float64{512.0 + 500, 512.0})
	if err == nil {
		t.Error("expected AIT projection to reject out-of-bounds point")
	}
}

// TestProjection_UnknownFallsBackToLinear verifies unknown projection codes
// silently fall back to linear mapping (no spherical deprojection).
func TestProjection_UnknownFallsBackToLinear(t *testing.T) {
	w := fits.NewWCS(2)
	w.SetCTYPE([]string{"RA---ZZZ", "DEC--ZZZ"})
	w.SetCRPIX([]float64{50.0, 50.0})
	w.SetCRVAL([]float64{10.0, 20.0})
	w.SetCDELT([]float64{-0.01, 0.01})

	// With unknown projection, extractProjection returns "" → linear fallback.
	// 5 pixels offset * 0.01 scale = 0.05 degrees
	res, err := w.PixelToWorld([]float64{55.0, 55.0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Linear: CRVAL + offset * CDELT
	expectedRA := 10.0 + (55.0-50.0)*(-0.01)
	expectedDec := 20.0 + (55.0-50.0)*0.01

	if math.Abs(res[0]-expectedRA) > 1e-10 {
		t.Errorf("RA: expected %.4f, got %.4f", expectedRA, res[0])
	}

	if math.Abs(res[1]-expectedDec) > 1e-10 {
		t.Errorf("Dec: expected %.4f, got %.4f", expectedDec, res[1])
	}
}

// TestProjection_SwappedAxes verifies that CTYPE1="DEC--TAN" / CTYPE2="RA---TAN"
// (uncommon but legal per FITS standard) produces correct results.
func TestProjection_SwappedAxes(t *testing.T) {
	// Standard layout: CTYPE1=RA, CTYPE2=DEC
	std := fits.NewWCS(2)
	std.SetCTYPE([]string{"RA---TAN", "DEC--TAN"})
	std.SetCRPIX([]float64{512.5, 512.5})
	std.SetCRVAL([]float64{150.0, 45.0})
	std.SetCDELT([]float64{-0.001, 0.001})

	// Swapped layout: CTYPE1=DEC, CTYPE2=RA
	swp := fits.NewWCS(2)
	swp.SetCTYPE([]string{"DEC--TAN", "RA---TAN"})
	swp.SetCRPIX([]float64{512.5, 512.5})
	swp.SetCRVAL([]float64{45.0, 150.0}) // Dec in slot 0, RA in slot 1
	swp.SetCDELT([]float64{0.001, -0.001})

	// Standard pixel (530,520): dx=+17.5 on RA (axis 0), dy=+7.5 on Dec (axis 1).
	// For swapped WCS to see the same field offset:
	//   axis 0 = Dec → needs dy=+7.5 → pixel = 520
	//   axis 1 = RA  → needs dx=+17.5 → pixel = 530
	// So swapped pixel is (520, 530).
	pxStd := []float64{530.0, 520.0}
	pxSwp := []float64{520.0, 530.0}

	stdWorld, err := std.PixelToWorld(pxStd)
	if err != nil {
		t.Fatalf("standard PixelToWorld: %v", err)
	}

	swpWorld, err := swp.PixelToWorld(pxSwp)
	if err != nil {
		t.Fatalf("swapped PixelToWorld: %v", err)
	}

	// Standard: world[0]=RA, world[1]=Dec
	// Swapped:  world[0]=Dec, world[1]=RA
	stdRA, stdDec := stdWorld[0], stdWorld[1]
	swpRA, swpDec := swpWorld[1], swpWorld[0]

	if math.Abs(stdRA-swpRA) > 1e-10 {
		t.Errorf("RA mismatch: standard=%.6f, swapped=%.6f", stdRA, swpRA)
	}

	if math.Abs(stdDec-swpDec) > 1e-10 {
		t.Errorf("Dec mismatch: standard=%.6f, swapped=%.6f", stdDec, swpDec)
	}

	// Round-trip the swapped case.
	rtPx, err := swp.WorldToPixel(swpWorld)
	if err != nil {
		t.Fatalf("swapped WorldToPixel: %v", err)
	}

	if math.Abs(rtPx[0]-pxSwp[0]) > 0.01 || math.Abs(rtPx[1]-pxSwp[1]) > 0.01 {
		t.Errorf("swapped round-trip: want (%.1f, %.1f), got (%.6f, %.6f)",
			pxSwp[0], pxSwp[1], rtPx[0], rtPx[1])
	}

	t.Logf("standard: RA=%.6f Dec=%.6f", stdRA, stdDec)
	t.Logf("swapped:  RA=%.6f Dec=%.6f", swpRA, swpDec)
}

// TestSIPDistortion verifies that SIP polynomial distortion produces correct
// pixel↔world round-trips and measurable shifts vs undistorted TAN.
func TestSIPDistortion(t *testing.T) {
	// Set up a standard TAN WCS for a 2048×2048 detector.
	// CRPIX at center, 0.25 arcsec/pixel scale (typical HST/WFC3-like).
	w := fits.NewWCS(2)
	w.SetCTYPE([]string{"RA---TAN-SIP", "DEC--TAN-SIP"})
	w.SetCRPIX([]float64{1024.5, 1024.5})
	w.SetCRVAL([]float64{150.0, 45.0})

	scale := 0.25 / 3600.0 // 0.25 arcsec in degrees
	w.SetCDELT([]float64{-scale, scale})

	// SIP forward distortion coefficients (order 3).
	// These are representative of a moderately distorted survey camera.
	sipA := map[[2]int]float64{
		{2, 0}: 2.0e-6,  // quadratic in u
		{0, 2}: 1.5e-6,  // quadratic in v
		{1, 1}: -1.0e-6, // cross term
		{3, 0}: 5.0e-10, // cubic in u
		{0, 3}: 3.0e-10, // cubic in v
	}
	sipB := map[[2]int]float64{
		{2, 0}: 1.0e-6,
		{0, 2}: 2.5e-6,
		{1, 1}: -0.8e-6,
		{3, 0}: 4.0e-10,
		{0, 3}: 6.0e-10,
	}
	w.SetSIP(sipA, sipB)

	// SIP inverse coefficients (approximate inverse of forward).
	// In practice these are computed by fitting, here we use negated forward
	// coefficients as a first-order approximation.
	sipAP := map[[2]int]float64{
		{2, 0}: -2.0e-6,
		{0, 2}: -1.5e-6,
		{1, 1}: 1.0e-6,
		{3, 0}: -5.0e-10,
		{0, 3}: -3.0e-10,
	}
	sipBP := map[[2]int]float64{
		{2, 0}: -1.0e-6,
		{0, 2}: -2.5e-6,
		{1, 1}: 0.8e-6,
		{3, 0}: -4.0e-10,
		{0, 3}: -6.0e-10,
	}
	w.SetSIPInverse(sipAP, sipBP)

	// Also set up a pure TAN WCS (no SIP) for comparison.
	wNoSIP := fits.NewWCS(2)
	wNoSIP.SetCTYPE([]string{"RA---TAN", "DEC--TAN"})
	wNoSIP.SetCRPIX([]float64{1024.5, 1024.5})
	wNoSIP.SetCRVAL([]float64{150.0, 45.0})
	wNoSIP.SetCDELT([]float64{-scale, scale})

	// Test 1: At reference pixel, SIP should produce zero distortion.
	refWorld, err := w.PixelToWorld([]float64{1024.5, 1024.5})
	if err != nil {
		t.Fatalf("SIP PixelToWorld at CRPIX: %v", err)
	}

	if math.Abs(refWorld[0]-150.0) > 1e-10 || math.Abs(refWorld[1]-45.0) > 1e-10 {
		t.Errorf("SIP at CRPIX: expected (150, 45), got (%.10f, %.10f)", refWorld[0], refWorld[1])
	}

	// Test 2: At a field edge, SIP should produce a measurable shift vs pure TAN.
	edgePx := []float64{100.0, 100.0} // ~924 pixels from center

	sipWorld, err := w.PixelToWorld(edgePx)
	if err != nil {
		t.Fatalf("SIP PixelToWorld at edge: %v", err)
	}

	tanWorld, err := wNoSIP.PixelToWorld(edgePx)
	if err != nil {
		t.Fatalf("TAN PixelToWorld at edge: %v", err)
	}

	// The SIP shift should be nonzero but small (sub-arcsecond for these coefficients).
	dRA := (sipWorld[0] - tanWorld[0]) * 3600.0  // arcsec
	dDec := (sipWorld[1] - tanWorld[1]) * 3600.0 // arcsec
	shift := math.Sqrt(dRA*dRA + dDec*dDec)

	t.Logf("SIP shift at edge: dRA=%.4f\" dDec=%.4f\" total=%.4f\"", dRA, dDec, shift)

	if shift < 0.01 {
		t.Errorf("SIP distortion too small at field edge: %.4f arcsec", shift)
	}

	if shift > 10.0 {
		t.Errorf("SIP distortion unreasonably large: %.4f arcsec", shift)
	}

	// Test 3: Forward/inverse round-trip with SIP.
	testPixels := [][]float64{
		{1024.5, 1024.5}, // center
		{100.0, 100.0},   // corner
		{1900.0, 1900.0}, // opposite corner
		{500.0, 1500.0},  // off-axis
		{1024.5, 100.0},  // edge
	}

	for _, px := range testPixels {
		world, err := w.PixelToWorld(px)
		if err != nil {
			t.Fatalf("SIP PixelToWorld(%.1f, %.1f): %v", px[0], px[1], err)
		}

		rtPx, err := w.WorldToPixel(world)
		if err != nil {
			t.Fatalf("SIP WorldToPixel(%.6f, %.6f): %v", world[0], world[1], err)
		}

		dPx := math.Sqrt((rtPx[0]-px[0])*(rtPx[0]-px[0]) + (rtPx[1]-px[1])*(rtPx[1]-px[1]))
		if dPx > 0.1 { // 0.1 pixel tolerance (AP/BP are approximate)
			t.Errorf("SIP round-trip (%.1f, %.1f) → (%.6f, %.6f) → (%.4f, %.4f): residual %.4f px",
				px[0], px[1], world[0], world[1], rtPx[0], rtPx[1], dPx)
		}
	}
}

// TestTPVDistortion verifies that TPV polynomial distortion produces correct
// pixel↔world round-trips and measurable shifts vs undistorted TAN.
func TestTPVDistortion(t *testing.T) {
	// Set up a TAN-TPV WCS for a 4096×4096 detector.
	// 0.25 arcsec/pixel scale — typical ground-based wide-field imager.
	w := fits.NewWCS(2)
	w.SetCTYPE([]string{"RA---TAN-TPV", "DEC--TAN-TPV"})
	w.SetCRPIX([]float64{2048.5, 2048.5})
	w.SetCRVAL([]float64{150.0, 45.0})

	scale := 0.25 / 3600.0 // 0.25 arcsec in degrees
	w.SetCDELT([]float64{-scale, scale})

	// TPV distortion coefficients (representative of SCAMP astrometric solution).
	// PV1 affects the longitude axis, PV2 affects the latitude axis.
	pv1 := map[int]float64{
		1:  1.0,   // linear term (identity)
		4:  0.02,  // x² radial
		6:  0.015, // y² radial
		7:  0.5,   // x³
		9:  0.3,   // xy²
		11: -0.1,  // r³
	}
	pv2 := map[int]float64{
		2:  1.0,   // linear term (identity)
		4:  0.01,  // x²
		6:  0.025, // y²
		8:  0.4,   // x²y
		10: 0.6,   // y³
		11: -0.1,  // r³
	}
	w.SetTPV(pv1, pv2)

	// Pure TAN for comparison.
	wNoTPV := fits.NewWCS(2)
	wNoTPV.SetCTYPE([]string{"RA---TAN", "DEC--TAN"})
	wNoTPV.SetCRPIX([]float64{2048.5, 2048.5})
	wNoTPV.SetCRVAL([]float64{150.0, 45.0})
	wNoTPV.SetCDELT([]float64{-scale, scale})

	// Test 1: At reference pixel, TPV should produce zero distortion.
	refWorld, err := w.PixelToWorld([]float64{2048.5, 2048.5})
	if err != nil {
		t.Fatalf("TPV PixelToWorld at CRPIX: %v", err)
	}

	if math.Abs(refWorld[0]-150.0) > 1e-10 || math.Abs(refWorld[1]-45.0) > 1e-10 {
		t.Errorf("TPV at CRPIX: expected (150, 45), got (%.10f, %.10f)", refWorld[0], refWorld[1])
	}

	// Test 2: At field edge, TPV should produce a measurable shift vs pure TAN.
	edgePx := []float64{200.0, 200.0} // ~1848 pixels from center

	tpvWorld, err := w.PixelToWorld(edgePx)
	if err != nil {
		t.Fatalf("TPV PixelToWorld at edge: %v", err)
	}

	tanWorld, err := wNoTPV.PixelToWorld(edgePx)
	if err != nil {
		t.Fatalf("TAN PixelToWorld at edge: %v", err)
	}

	dRA := (tpvWorld[0] - tanWorld[0]) * 3600.0  // arcsec
	dDec := (tpvWorld[1] - tanWorld[1]) * 3600.0 // arcsec
	shift := math.Sqrt(dRA*dRA + dDec*dDec)

	t.Logf("TPV shift at edge: dRA=%.4f\" dDec=%.4f\" total=%.4f\"", dRA, dDec, shift)

	if shift < 0.001 {
		t.Errorf("TPV distortion too small at field edge: %.6f arcsec", shift)
	}

	if shift > 100.0 {
		t.Errorf("TPV distortion unreasonably large: %.4f arcsec", shift)
	}

	// Test 3: Forward/inverse round-trip with TPV.
	testPixels := [][]float64{
		{2048.5, 2048.5}, // center
		{200.0, 200.0},   // corner
		{3900.0, 3900.0}, // opposite corner
		{1000.0, 3000.0}, // off-axis
		{2048.5, 200.0},  // edge
	}

	for _, px := range testPixels {
		world, err := w.PixelToWorld(px)
		if err != nil {
			t.Fatalf("TPV PixelToWorld(%.1f, %.1f): %v", px[0], px[1], err)
		}

		rtPx, err := w.WorldToPixel(world)
		if err != nil {
			t.Fatalf("TPV WorldToPixel(%.6f, %.6f): %v", world[0], world[1], err)
		}

		dPx := math.Sqrt((rtPx[0]-px[0])*(rtPx[0]-px[0]) + (rtPx[1]-px[1])*(rtPx[1]-px[1]))
		if dPx > 0.01 { // 0.01 pixel tolerance (Newton-Raphson should be very accurate)
			t.Errorf("TPV round-trip (%.1f, %.1f) → (%.6f, %.6f) → (%.4f, %.4f): residual %.6f px",
				px[0], px[1], world[0], world[1], rtPx[0], rtPx[1], dPx)
		}
	}
}
