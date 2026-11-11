package simbad_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/catalog/simbad"
)

// A non-finite number in a SIMBAD answer is read as an empty cell is (#1080).
//
// strconv.ParseFloat accepts NaN and the infinities, and this parser skipped
// only an empty cell. A NaN position made a target that never rose, and a NaN
// proper motion a star that left its position on every date but its epoch.
func TestParseCSVReadsNonFiniteNumbersAsAbsent(t *testing.T) {
	t.Parallel()

	const header = "oid,main_id,ra,dec,otype,otype_path,pmra,pmdec,plx_value,rvz_radvel,V\n"

	targets, err := simbad.ParseCSV(strings.NewReader(header +
		"1,* nan ra,NaN,-16.71611,*,*,1,2,3,4,5\n" +
		"2,* inf dec,101.28715,Infinity,*,*,1,2,3,4,5\n" +
		"3,* dec past the pole,101.28715,95,*,*,1,2,3,4,5\n" +
		"4,* nan extras,101.28715,-16.71611,*,*,NaN,Inf,NaN,-Inf,NaN\n" +
		"5,* real,101.28715,-16.71611,*,*,-546.01,-1223.07,379.21,-5.5,-1.46\n"))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}

	byID := map[string]resolve.Target{}
	for _, tgt := range targets {
		byID[tgt.ID] = tgt
	}

	for _, id := range []string{"* nan ra", "* inf dec", "* dec past the pole"} {
		tgt, ok := byID[id]
		if !ok {
			t.Fatalf("%q is missing; a target with no usable position is still a name SIMBAD knows", id)
		}

		if tgt.HasCoord {
			t.Errorf("%q: HasCoord is true with coordinate %v; a non-finite or impossible "+
				"position is no position", id, tgt.Coord)
		}
	}

	extras := byID["* nan extras"]
	if !extras.HasCoord {
		t.Fatal(`"* nan extras" lost its real position`)
	}

	if extras.PmRA.Radians() != 0 || extras.PmDec.Radians() != 0 || extras.Parallax.Radians() != 0 {
		t.Errorf("non-finite kinematics were kept: pmRA %v, pmDec %v, parallax %v",
			extras.PmRA, extras.PmDec, extras.Parallax)
	}

	if extras.HasRadialVelocity || extras.HasVMag {
		t.Errorf("a non-finite radial velocity or magnitude was kept: HasRadialVelocity %t, HasVMag %t",
			extras.HasRadialVelocity, extras.HasVMag)
	}

	// The control: the same columns, real, are all read.
	control := byID["* real"]
	if !control.HasCoord || control.PmRA.Radians() == 0 || control.Parallax.Radians() == 0 ||
		!control.HasRadialVelocity || !control.HasVMag || control.VMag != -1.46 {
		t.Errorf("the real row was not read in full: %+v", control)
	}
}
