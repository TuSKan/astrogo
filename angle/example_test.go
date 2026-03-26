package angle_test

import (
	"fmt"
	"github.com/TuSKan/astrogo/angle"
)

func ExampleAngle_DMSString() {
	a := angle.Deg(12.5825)
	fmt.Println(a.DMSString(1))
	// Output: +12°34'57.0"
}

func ExampleAngle_HMSString() {
	a := angle.Deg(180)
	fmt.Println(a.HMSString(0))
	// Output: 12h00m00s
}

func ExampleParseDMS() {
	a, _ := angle.ParseDMS("+12:34:57")
	fmt.Printf("%.4f\n", a.Degrees())
	// Output: 12.5825
}

func ExampleAngle_Wrap360() {
	a := angle.Deg(370)
	fmt.Printf("%.0f\n", a.Wrap360().Degrees())
	// Output: 10
}
