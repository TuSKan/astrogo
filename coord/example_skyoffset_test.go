package coord_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// A five-point dither pattern around a target: the offsets are what an
// observer writes down, and the frame turns each one into the sky position the
// telescope is actually sent to.
func ExampleSkyOffset() {
	target := coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911)) // M42
	f := coord.NewSkyOffset(target, 0)

	// 20 arcseconds, centre plus the four corners.
	const d = 20.0 / 3600

	for _, p := range []struct{ lon, lat float64 }{
		{0, 0}, {+d, +d}, {-d, +d}, {-d, -d}, {+d, -d},
	} {
		pointing := f.ToICRS(angle.Deg(p.lon), angle.Deg(p.lat))

		fmt.Printf("offset (%+.0f\", %+.0f\") -> RA %s Dec %s\n",
			p.lon*3600, p.lat*3600,
			pointing.RA().HMSString(2), pointing.Dec().DMSString(1))
	}
	// The east-west pairs differ by 2.68 seconds of right ascension rather
	// than by 2.67, because 40 arcseconds of arc is 40/cos(dec) arcseconds of
	// right ascension. Applying that is the frame's job, not the caller's.

	// Output:
	// offset (+0", +0") -> RA 05h35m17.30s Dec -05°23'28.0"
	// offset (+20", +20") -> RA 05h35m18.64s Dec -05°23'08.0"
	// offset (-20", +20") -> RA 05h35m15.96s Dec -05°23'08.0"
	// offset (-20", -20") -> RA 05h35m15.96s Dec -05°23'48.0"
	// offset (+20", -20") -> RA 05h35m18.64s Dec -05°23'48.0"
}

// The frame's rotation is the position angle its +lat axis points at, so
// setting it to a slit's position angle makes lat the along-slit coordinate
// and lon the across-slit one.
func ExampleSkyOffset_rotation() {
	target := coord.NewICRS(angle.Deg(210.8023), angle.Deg(54.3488))

	// A companion 12 arcseconds from the target at position angle 145°. In a
	// frame turned to that position angle it lies straight up the +lat axis,
	// which is what makes the rotation worth having: a slit laid at 145° has
	// the companion on axis and nothing to resolve across it.
	slit := coord.NewSkyOffset(target, angle.Deg(145))
	companion := slit.ToICRS(0, angle.Arcsec(12))

	across, along := slit.FromICRS(companion)
	fmt.Printf("across the slit %+.2f\", along it %+.2f\"\n",
		across.Arcseconds(), along.Arcseconds())

	// The same star seen north-up, where 145° is mostly south and a little
	// east: 12·sin 145° and 12·cos 145°.
	east, north := coord.NewSkyOffset(target, 0).FromICRS(companion)
	fmt.Printf("east %+.2f\", north %+.2f\"\n", east.Arcseconds(), north.Arcseconds())

	// Output:
	// across the slit +0.00", along it +12.00"
	// east +6.88", north -9.83"
}
