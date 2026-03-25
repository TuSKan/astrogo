package main

import (
	"fmt"

	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/optics"
)

func main() {
	// 1. Define the hardware profile structurally
	sensor := optics.Sensor{WidthMM: 35.8, HeightMM: 23.9, PixelPitch: 6.54}

	// Optics: 70-200mm lens maxed out, utilizing a 2x teleconverter natively
	rig := optics.Optics{FocalLengthMM: 200.0, Extender: 2.0}

	// 2. Calculate the boundaries of the glass explicitly
	fovX, fovY := optics.CalculateFOV(sensor, rig)
	fmt.Printf("Rig FOV: %.2f° x %.2f°\n", fovX, fovY)

	// 3. Check a massive target (The Andromeda Galaxy / M31 explicit bounding)
	target, err := catalog.Lookup("NGC0224")
	if err != nil {
		fmt.Printf("Error tracking target intrinsically: %v\n", err)
		return
	}

	// Catalog stores dimensions in arcminutes, mapping seamlessly back to degrees
	targetSizeX := target.MajorAxis / 60.0
	targetSizeY := target.MinorAxis / 60.0

	fmt.Printf("Target %s Size: %.2f° x %.2f°\n", target.ID, targetSizeX, targetSizeY)

	// 4. Evaluate the fit against mapped properties
	if targetSizeX > fovX || targetSizeY > fovY {
		fmt.Println("Warning: Target exceeds FOV. A mosaic is required.")
	} else {
		fmt.Println("Target fits perfectly within the frame.")
	}
}
