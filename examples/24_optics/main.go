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
	"github.com/TuSKan/astrogo/examples/internal/show"
	"github.com/TuSKan/astrogo/optics"
	"github.com/TuSKan/astrogo/unit"
)

func main() {
	scope, err := optics.NewTelescope(unit.Millimeters(203), unit.Millimeters(2032)) // 8" f/10 SCT
	if err != nil {
		log.Fatalf("telescope: %v", err)
	}

	show.Title(`Optical system: an 8" f/10 Schmidt-Cassegrain`, "optics.Telescope, Eyepiece and Sensor")

	show.Section("The telescope")
	show.Fields(
		show.KV("Aperture", "%.0f mm", scope.Aperture().Millimeters()),
		show.KV("Focal length", "%.0f mm", scope.FocalLength().Millimeters()),
		show.KV("Focal ratio", "f/%.0f", scope.FocalRatio()),
		show.KV("Dawes limit", `%.2f"`, scope.DawesLimit().Arcseconds()),
		show.KV("Max useful magnification", "%.0fx", scope.MaxUsefulMagnification()),
		show.KV("Limiting magnitude", "%.1f", scope.LimitingMagnitude()),
	)

	// ── Wide-field eyepiece, exact TrueFOV via a known field stop ──────────
	wideField, err := optics.NewEyepiece(unit.Millimeters(32), angle.Deg(52), optics.WithFieldStop(unit.Millimeters(27.4)))
	if err != nil {
		log.Fatalf("wide-field eyepiece: %v", err)
	}

	// ── Planetary eyepiece, no field stop → AFOV/magnification fallback ────
	planetary, err := optics.NewEyepiece(unit.Millimeters(9), angle.Deg(52))
	if err != nil {
		log.Fatalf("planetary eyepiece: %v", err)
	}

	// ── Same eyepiece behind a 2x Barlow ────────────────────────────────────
	barlowed, err := scope.WithBarlow(2)
	if err != nil {
		log.Fatalf("barlow: %v", err)
	}

	show.Section("Eyepieces")

	tab := show.NewTable(show.Left("Eyepiece"), show.Right("Magnification"),
		show.Right("True field"), show.Right("Exit pupil"))

	for _, c := range []struct {
		label    string
		scope    optics.Telescope
		eyepiece optics.Eyepiece
	}{
		{"32 mm, 52°, 27.4 mm field stop", scope, wideField},
		{"9 mm, 52°", scope, planetary},
		{"9 mm, 52°, 2x Barlow", barlowed, planetary},
	} {
		tab.Row(c.label,
			fmt.Sprintf("%.0fx", c.scope.Magnification(c.eyepiece)),
			fmt.Sprintf("%.2f°", c.scope.TrueFOV(c.eyepiece).Degrees()),
			fmt.Sprintf("%.2f mm", c.scope.ExitPupil(c.eyepiece).Millimeters()))
	}

	tab.Print()
	fmt.Println()
	show.Note("A known field stop gives the true field exactly. Without one it",
		"is the apparent field over the magnification, an approximation",
		"that drifts on wide-field designs.")

	// ── Imaging: CMOS sensor field of view and plate scale ─────────────────
	sensor := optics.Sensor{
		Width:      unit.Millimeters(23.5),
		Height:     unit.Millimeters(15.6),
		PixelPitch: unit.Millimeters(3.76 / 1000), // 3.76 µm
	}

	w, h := scope.SensorFOV(sensor)

	show.Section("Imaging: a 23.5 × 15.6 mm sensor, 3.76 µm pixels")
	show.Fields(
		show.KV("Pixel scale", `%.2f"/pixel`, scope.PixelScale(sensor).Arcseconds()),
		show.KV("Field", "%.2f' × %.2f'", w.Arcminutes(), h.Arcminutes()),
	)
}
