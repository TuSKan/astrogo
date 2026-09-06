package norad_test

import (
	"encoding/json"
	"testing"

	"github.com/TuSKan/astrogo/catalog/norad"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
)

// This file fuzzes the CelesTrak GP pipeline: JSON off the wire, into a GP,
// back out as a TLE, and into the SGP4 constructor. Every seed is a string
// literal — no checked-in fixture — so the seed corpus runs under
// `go test ./...` and is in every CI run for free. Extended fuzzing is manual
// and periodic; see CLAUDE.md, including the -fuzzminimizetime flag (#140).
//
// # Why the whole chain rather than the unmarshal
//
// encoding/json is not what needs fuzzing. What does is GP.ToTLE, which takes
// sixteen numbers a remote service supplied and formats them into fixed columns
// — padding, truncating, exponent-encoding and computing a checksum. A value
// outside the range those columns can hold does not fail there; it produces a
// TLE that is the wrong length, or numeric in the wrong places, and hands it to
// a propagator that has already been shown to exit the process and to panic on
// input of exactly that shape (see satellite's fuzz targets and #120).
//
// So the property is end to end: no GP that unmarshals may produce a TLE that
// crashes the constructor. If ToTLE emits something malformed, ValidateTLE has
// to be the thing that says so.
func FuzzGPToTLE(f *testing.F) {
	// A real CelesTrak GP response for the ISS, one element set.
	f.Add(`{"OBJECT_NAME":"ISS (ZARYA)","OBJECT_ID":"1998-067A",` +
		`"EPOCH":"2026-04-19T11:45:32.156928","MEAN_MOTION":15.50103472,` +
		`"ECCENTRICITY":0.0002921,"INCLINATION":51.6376,"RA_OF_ASC_NODE":247.4627,` +
		`"ARG_OF_PERICENTER":130.536,"MEAN_ANOMALY":325.0288,"EPHEMERIS_TYPE":0,` +
		`"CLASSIFICATION_TYPE":"U","NORAD_CAT_ID":25544,"ELEMENT_SET_NO":999,` +
		`"REV_AT_EPOCH":50123,"BSTAR":0.00023479,"MEAN_MOTION_DOT":0.00016717,` +
		`"MEAN_MOTION_DDOT":0}`)

	// The numeric extremes the fixed-width columns cannot represent: each one
	// is a value a hostile or broken service can legally put in JSON.
	f.Add(`{"NORAD_CAT_ID":999999999,"MEAN_MOTION":1e308,"ECCENTRICITY":-1,` +
		`"INCLINATION":1e308,"BSTAR":-1e308,"MEAN_MOTION_DOT":1e-308,` +
		`"OBJECT_ID":"","EPOCH":""}`)
	f.Add(`{"MEAN_MOTION":0,"ECCENTRICITY":0,"INCLINATION":0,"NORAD_CAT_ID":0,` +
		`"EPOCH":"0000-00-00T00:00:00"}`)

	// A long designation, since formatIntDes splits and pads it.
	f.Add(`{"OBJECT_ID":"1998-067AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","NORAD_CAT_ID":1}`)

	f.Add(`{}`)
	f.Add(``)

	f.Fuzz(func(t *testing.T, body string) {
		var gp norad.GP
		if err := json.Unmarshal([]byte(body), &gp); err != nil {
			return
		}

		// EpochTime is the other place a service-supplied string is parsed.
		_, _ = gp.EpochTime()

		line1, line2 := gp.ToTLE()

		// ToTLE has no error return, so the contract it can be held to is that
		// whatever it produces, the checker in front of SGP4 either refuses or
		// survives. A crash between here and the end of this function is the
		// finding.
		if err := satellite.ValidateTLE(line1, line2); err != nil {
			return
		}

		sat, err := satellite.NewFromTLE(gp.ObjectName, line1, line2)
		if err != nil {
			return
		}

		if sat == nil {
			t.Fatal("NewFromTLE returned a nil satellite and a nil error")
		}

		_ = sat.OrbitalPeriod()
	})
}
