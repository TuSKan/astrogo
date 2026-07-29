package optics_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/optics"
)

// Example computes magnification, true field of view, and exit pupil for
// a 200mm f/10 telescope paired with a 25mm, 68°-apparent-field eyepiece.
func Example() {
	scope, err := optics.NewTelescope(200, 2000) // 200mm aperture, 2000mm focal length
	if err != nil {
		fmt.Println(err)
		return
	}

	eyepiece, err := optics.NewEyepiece(25, angle.Deg(68))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Focal ratio: f/%.0f\n", scope.FocalRatio())
	fmt.Printf("Magnification: %.0fx\n", scope.Magnification(eyepiece))
	fmt.Printf("True field of view: %.2f°\n", scope.TrueFOV(eyepiece).Degrees())
	fmt.Printf("Exit pupil: %.1f mm\n", scope.ExitPupil(eyepiece))

	// Output:
	// Focal ratio: f/10
	// Magnification: 80x
	// True field of view: 0.85°
	// Exit pupil: 2.5 mm
}
