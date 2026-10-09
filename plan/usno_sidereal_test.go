package plan_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// usnoSiderealTolerance is two of the reference's printing steps.
//
// USNO prints sidereal time to 0.0001 s, so rounding alone puts the reference
// up to half a step from the value it stands for. Both sides implement IAU
// 2006/2000A sidereal time, where two implementations agree to microseconds,
// so the second step is room for that and nothing else. What the bound is
// built to catch is far larger: GMST in place of GAST differs by the equation
// of the equinoxes, 0.36 to 0.97 s at these epochs, and a longitude applied
// with the wrong sign by hours.
const usnoSiderealTolerance = 2e-4 // seconds of time

// TestUSNO_SiderealTime holds Greenwich and local apparent sidereal time to
// USNO's.
//
// The reference is the USNO Astronomical Applications API, v4.0.1,
// /api/siderealtime, which computes with NOVAS, independently of the SOFA
// routines astrogo reaches through gofa. It takes the instant as UT1 and
// defines local apparent sidereal time as GAST plus east longitude with no
// polar motion, which is Site.LocalSiderealTime's definition, so the two are
// compared like for like. The epochs run from 1990 to 2049, and the sites lie
// east and west of Greenwich and in both hemispheres. Values are transcribed
// from the response's decimal strings, fetched 2026-10-08.
//
// GAST takes TT as well as UT1, for precession and nutation, and TT is reached
// from UT1 through DUT1. Without EOP that is up to 0.9 s off, which moves
// GAST by microarcseconds, so this holds with or without them.
//
// Until #670 nothing held sidereal time tighter than 0.5 degrees, two minutes
// of time, and the test of this name asserted only that the result lay in
// [0, 360).
func TestUSNO_SiderealTime(t *testing.T) {
	cases := []struct {
		site       string
		lat, lon   float64 // degrees, east positive
		date, ut1  string  // as sent to USNO
		gast, last string  // as USNO returned them
	}{
		{"Greenwich", 51.4778, 0, "1990-01-01", "00:00:00", "06:41:32.7950", "06:41:32.7950"},
		{"Siding Spring", -31.2733, 149.0617, "2000-01-01", "12:00:00", "18:41:49.6974", "04:38:04.5054"},
		{"Mauna Kea", 19.8261, -155.47, "2012-07-01", "06:30:00", "01:08:52.5597", "14:46:59.7597"},
		{"São Paulo", -23.600833, -46.6525, "2026-04-06", "21:00:00", "10:00:38.7043", "06:54:02.1043"},
		{"Paranal", -24.6272, -70.4042, "2040-10-15", "18:45:30", "20:25:08.1286", "15:43:31.1206"},
		{"La Palma", 28.7606, -17.8792, "2049-12-31", "23:00:00", "05:43:14.0413", "04:31:43.0333"},
	}

	for _, c := range cases {
		t.Run(c.site+" "+c.date, func(t *testing.T) {
			epoch := usnoUT1(t, c.date, c.ut1)

			geodetic, err := coord.NewGeodetic(angle.Deg(c.lon), angle.Deg(c.lat), 0)
			if err != nil {
				t.Fatalf("NewGeodetic: %v", err)
			}

			site, err := plan.NewSite(c.site, geodetic)
			if err != nil {
				t.Fatalf("NewSite: %v", err)
			}

			gast, err := epoch.GAST()
			if err != nil {
				t.Fatalf("GAST: %v", err)
			}

			last, err := site.LocalSiderealTime(epoch)
			if err != nil {
				t.Fatalf("LocalSiderealTime: %v", err)
			}

			for _, q := range []struct {
				name string
				got  angle.Angle
				want string
			}{
				{"GAST", gast, c.gast},
				{"LAST", last, c.last},
			} {
				diff := sideTimeDiffSeconds(q.got, hmsHours(t, q.want))

				t.Logf("%s %s: astrogo %s, USNO %s, %+.5f s", c.date, q.name, q.got.HMSString(4), q.want, diff)

				if diff > usnoSiderealTolerance || diff < -usnoSiderealTolerance {
					t.Errorf("%s at %s %s UT1: %+.5f s from USNO's %s, beyond %.4f s",
						q.name, c.date, c.ut1, diff, q.want, usnoSiderealTolerance)
				}
			}
		})
	}
}

// usnoUT1 returns the instant USNO was asked about. Its date and time are UT1,
// so the calendar reading is converted to a Julian Date and labeled UT1
// rather than converted from UTC. None of these dates holds a leap second, so
// the calendar arithmetic is the same on both scales.
func usnoUT1(t *testing.T, date, clock string) time.Time {
	t.Helper()

	cal, err := time.Parse("2006-01-02 15:04:05", date+" "+clock)
	if err != nil {
		t.Fatalf("parse %s %s: %v", date, clock, err)
	}

	jd1, jd2 := time.FromGo(cal).JDParts()

	return time.FromJDParts(jd1, jd2, time.UT1)
}

// hmsHours reads USNO's "hh:mm:ss.ssss" as hours.
func hmsHours(t *testing.T, s string) float64 {
	t.Helper()

	fields := strings.Split(s, ":")
	if len(fields) != 3 {
		t.Fatalf("%q is not hh:mm:ss", s)
	}

	var parts [3]float64

	for i, f := range fields {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}

		parts[i] = v
	}

	return parts[0] + parts[1]/60 + parts[2]/3600
}

// sideTimeDiffSeconds is got minus want in seconds of time, taken the short
// way round the 24-hour circle.
func sideTimeDiffSeconds(got angle.Angle, wantHours float64) float64 {
	diff := (got.Hours() - wantHours) * 3600

	switch {
	case diff > 43200:
		diff -= 86400
	case diff < -43200:
		diff += 86400
	}

	return diff
}
