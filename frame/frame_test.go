package frame_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/frame"
	"github.com/TuSKan/astrogo/time"
)

func TestFrameIdentity(t *testing.T) {
	if frame.ICRSFrame.Name() != "ICRS" {
		t.Errorf("ICRSFrame name = %q, want ICRS", frame.ICRSFrame.Name())
	}
	if frame.GalacticFrame.Name() != "Galactic" {
		t.Errorf("GalacticFrame name = %q, want Galactic", frame.GalacticFrame.Name())
	}
}

func TestFrameEquality(t *testing.T) {
	icrs := frame.ICRS{}
	gal := frame.Galactic{}

	if !frame.Equals(icrs, icrs) {
		t.Error("ICRS should equal itself")
	}
	if frame.Equals(icrs, gal) {
		t.Error("ICRS should not equal Galactic")
	}

	// Ecliptic with metadata
	t1 := time.FromJD(2451545.0, time.UTC)
	t2 := time.FromJD(2460000.0, time.UTC)

	ec1 := frame.Ecliptic{Equinox: t1}
	ec2 := frame.Ecliptic{Equinox: t1}
	ec3 := frame.Ecliptic{Equinox: t2}

	if !frame.Equals(ec1, ec2) {
		t.Error("Ecliptic(T1) should equal Ecliptic(T1)")
	}
	if frame.Equals(ec1, ec3) {
		t.Error("Ecliptic(T1) should not equal Ecliptic(T2)")
	}

	// AltAz with metadata
	loc1 := frame.ObserversLocation{Lat: angle.Deg(45), Lon: angle.Deg(10)}
	loc2 := frame.ObserversLocation{Lat: angle.Deg(48), Lon: angle.Deg(11)}

	aa1 := frame.AltAz{Time: t1, Location: loc1}
	aa2 := frame.AltAz{Time: t1, Location: loc1}
	aa3 := frame.AltAz{Time: t1, Location: loc2}
	aa4 := frame.AltAz{Time: t2, Location: loc1}

	if !frame.Equals(aa1, aa2) {
		t.Error("AltAz(T1, L1) should equal itself")
	}
	if frame.Equals(aa1, aa3) {
		t.Error("AltAz(T1, L1) should not equal AltAz(T1, L2)")
	}
	if frame.Equals(aa1, aa4) {
		t.Error("AltAz(T1, L1) should not equal AltAz(T2, L1)")
	}
}

func TestString(t *testing.T) {
	icrs := frame.ICRS{}
	if icrs.String() != "ICRS" {
		t.Errorf("String: got %q, want ICRS", icrs.String())
	}
}
