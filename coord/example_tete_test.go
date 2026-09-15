package coord_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// Two systems both called "apparent", twenty arcminutes apart. The pipeline
// produces the CIRS one; an almanac prints the other.
func ExampleContext_ApparentToTETE() {
	ctx := coord.NewContext(
		time.Date(2026, 4, 15, 22, 0, 0, 0, time.LocationUTC),
		coord.MustGeodetic(angle.Deg(-70.40417), angle.Deg(-24.62722), 2635),
		atmosphere.Refraction{},
	)

	// Where the pipeline leaves a target: a CIRS place.
	cirs := ctx.AstrometricToApparent(
		coord.NewAstrometric(angle.Deg(83.8221), angle.Deg(-5.3911)),
	)

	// The same place, with right ascension measured from the true equinox.
	tete := ctx.ApparentToTETE(cirs)

	fmt.Printf("CIRS  RA %s Dec %s\n", cirs.RA().HMSString(2), cirs.Dec().DMSString(1))
	fmt.Printf("TETE  RA %s Dec %s\n", tete.RA().HMSString(2), tete.Dec().DMSString(1))
	fmt.Printf("apart by %.2f arcmin of RA, %.2f arcsec of Dec\n",
		tete.RA().Sub(cirs.RA()).Wrap180().Arcseconds()/60,
		tete.Dec().Sub(cirs.Dec()).Arcseconds())

	// Output:
	// CIRS  RA 05h35m13.33s Dec -05°22'32.6"
	// TETE  RA 05h36m34.51s Dec -05°22'32.6"
	// apart by 20.30 arcmin of RA, 0.00 arcsec of Dec
}
