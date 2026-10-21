package core

import (
	"testing"
)

// TestIDsAreNAIFCodes pins every named ID to the NAIF code it is a name for
// (#809). An ID used to be astrogo's own number, 1 to 12, which NAIF uses for
// the system barycenters: Pluto, 9, was served as the Pluto barycenter by
// coincidence and the Solar System barycenter, 12, was refused. The values
// here are NAIF's, from its integer ID codes, so a renumbering cannot pass.
func TestIDsAreNAIFCodes(t *testing.T) {
	for id, want := range map[ID]int32{
		SolarSystemBarycenter: 0,
		MercuryBarycenter:     1,
		VenusBarycenter:       2,
		EarthMoonBarycenter:   3,
		MarsBarycenter:        4,
		JupiterBarycenter:     5,
		SaturnBarycenter:      6,
		UranusBarycenter:      7,
		NeptuneBarycenter:     8,
		PlutoBarycenter:       9,
		Sun:                   10,
		Mercury:               199,
		Venus:                 299,
		Moon:                  301,
		Earth:                 399,
		Mars:                  499,
		Jupiter:               599,
		Saturn:                699,
		Uranus:                799,
		Neptune:               899,
		Pluto:                 999,
	} {
		if int32(id) != want {
			t.Errorf("%v = %d, want NAIF %d", id, int32(id), want)
		}
	}
}

// TestBodyIsThePlanetOfABarycenter: a system barycenter's Body is its planet's
// center, NAIF's 100n + 99, and every other ID is its own.
func TestBodyIsThePlanetOfABarycenter(t *testing.T) {
	for in, want := range map[ID]ID{
		MercuryBarycenter: Mercury,
		VenusBarycenter:   Venus,
		MarsBarycenter:    Mars,
		JupiterBarycenter: Jupiter,
		SaturnBarycenter:  Saturn,
		UranusBarycenter:  Uranus,
		NeptuneBarycenter: Neptune,
		PlutoBarycenter:   Pluto,

		Jupiter:               Jupiter,
		Sun:                   Sun,
		Moon:                  Moon,
		SolarSystemBarycenter: SolarSystemBarycenter,
		// NAIF pairs 3, the Earth-Moon barycenter, with 399 as it pairs 5
		// with 599.
		EarthMoonBarycenter: Earth,
	} {
		if got := in.Body(); got != want {
			t.Errorf("%v.Body() = %v, want %v", in, got, want)
		}
	}
}

// TestSatelliteIDIsNAIFsForANORADNumber: an Earth satellite's code is
// −100000 − its NORAD catalog number, as NAIF documents for a satellite
// without a DSN code, and the number comes back out of it.
func TestSatelliteIDIsNAIFsForANORADNumber(t *testing.T) {
	iss, ok := SatelliteID(25544)
	if !ok || iss != -125544 {
		t.Fatalf("SatelliteID(25544) = %d, %v; want -125544, true", iss, ok)
	}

	if n, ok := iss.SatelliteNumber(); !ok || n != 25544 {
		t.Errorf("SatelliteNumber() = %d, %v; want 25544, true", n, ok)
	}

	if got := iss.String(); got != "Satellite(25544)" {
		t.Errorf("String() = %q", got)
	}

	for _, n := range []int{0, -1} {
		if id, ok := SatelliteID(n); ok {
			t.Errorf("SatelliteID(%d) = %d, true; want not a catalog number", n, id)
		}
	}

	// Neither a named body nor a small body is a satellite.
	eros, _ := SmallBodyID(433)

	for _, id := range []ID{SolarSystemBarycenter, Sun, Earth, Moon, eros, -82} {
		if n, ok := id.SatelliteNumber(); ok {
			t.Errorf("%v.SatelliteNumber() = %d, true", id, n)
		}
	}
}

// TestStringNamesTheCode: a named ID prints its name, and an unnamed code its
// number, so a log line says which point it was.
func TestStringNamesTheCode(t *testing.T) {
	for id, want := range map[ID]string{
		Jupiter:           "Jupiter",
		JupiterBarycenter: "JupiterBarycenter",
		Moon:              "Moon",
		501:               "NAIF(501)",
		-82:               "NAIF(-82)",
	} {
		if got := id.String(); got != want {
			t.Errorf("ID(%d).String() = %q, want %q", int32(id), got, want)
		}
	}
}
