// Package main demonstrates the optics package: pure equipment-optics
// arithmetic for a Telescope/Eyepiece/Sensor combination — no astrometry,
// no ephemeris, no network access. Works entirely offline.
//
// Scenario: an 8" (203mm) f/10 Schmidt-Cassegrain (2032mm focal length),
// a common amateur setup, paired with a wide-field low-power eyepiece
// (with a known field-stop diameter, for an exact true-field-of-view
// figure), a high-power planetary eyepiece (no field stop supplied, so
// TrueFOV falls back to the apparent-field/magnification approximation),
// the same eyepiece behind a 2x Barlow, and a CMOS camera sensor for an
// imaging field-of-view/pixel-scale figure.
//
// Run: go run ./examples/24_optics/
package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/optics"
)

func main() {
	scope, err := optics.NewTelescope(203, 2032) // 8" f/10 SCT
	if err != nil {
		log.Fatalf("telescope: %v", err)
	}

	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  8\" f/10 SCT — OPTICAL SYSTEM FIGURES")
	fmt.Println("  AstroGo | optics.Telescope/Eyepiece/Sensor")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  Aperture           %.0f mm\n", scope.ApertureMM())
	fmt.Printf("  Focal length       %.0f mm\n", scope.FocalLengthMM())
	fmt.Printf("  Focal ratio        f/%.0f\n", scope.FocalRatio())
	fmt.Printf("  Dawes limit        %.2f\"\n", scope.DawesLimit().Arcseconds())
	fmt.Printf("  Max useful mag.    %.0fx\n", scope.MaxUsefulMagnification())
	fmt.Printf("  Limiting mag.      %.1f\n", scope.LimitingMagnitude())

	// ── Wide-field eyepiece, exact TrueFOV via a known field stop ──────────
	wideField, err := optics.NewEyepiece(32, angle.Deg(52), optics.WithFieldStop(27.4))
	if err != nil {
		log.Fatalf("wide-field eyepiece: %v", err)
	}

	printEyepiece(scope, "32mm / 52° (27.4mm field stop)", wideField)

	// ── Planetary eyepiece, no field stop → AFOV/magnification fallback ────
	planetary, err := optics.NewEyepiece(9, angle.Deg(52))
	if err != nil {
		log.Fatalf("planetary eyepiece: %v", err)
	}

	printEyepiece(scope, "9mm / 52° (no field stop — AFOV/mag. approximation)", planetary)

	// ── Same eyepiece behind a 2x Barlow ────────────────────────────────────
	barlowed, err := scope.WithBarlow(2)
	if err != nil {
		log.Fatalf("barlow: %v", err)
	}

	printEyepiece(barlowed, "9mm / 52°, behind a 2x Barlow", planetary)

	// ── Imaging: CMOS sensor field of view and plate scale ─────────────────
	sensor := optics.Sensor{WidthMM: 23.5, HeightMM: 15.6, PixelMicrons: 3.76}

	w, h := scope.SensorFOV(sensor)

	fmt.Println()
	fmt.Println("── Imaging: 23.5×15.6mm CMOS sensor, 3.76µm pixels ─────────────────")
	fmt.Println()
	fmt.Printf("  Pixel scale        %.2f\"/px\n", scope.PixelScale(sensor).Arcseconds())
	fmt.Printf("  Sensor field       %.2f' × %.2f'\n", w.Arcminutes(), h.Arcminutes())
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════")
}

func printEyepiece(scope optics.Telescope, label string, eyepiece optics.Eyepiece) {
	fmt.Println()
	fmt.Printf("── %s ──\n", label)
	fmt.Println()
	fmt.Printf("  Magnification      %.0fx\n", scope.Magnification(eyepiece))
	fmt.Printf("  True field of view %.2f°\n", scope.TrueFOV(eyepiece).Degrees())
	fmt.Printf("  Exit pupil         %.2f mm\n", scope.ExitPupil(eyepiece))
}
