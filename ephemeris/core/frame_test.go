package core

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestFrameString(t *testing.T) {
	cases := []struct {
		frame Frame
		want  string
	}{
		{FrameUnspecified, "unspecified"},
		{FrameICRS, "ICRS"},
		{FrameGCRS, "GCRS"},
		{FrameITRS, "ITRS"},
		{FrameTEME, "TEME"},
		{Frame(200), "Frame(200)"},
	}

	for _, c := range cases {
		if got := c.frame.String(); got != c.want {
			t.Errorf("Frame(%d).String() = %q, want %q", uint8(c.frame), got, c.want)
		}
	}
}

func TestCenterString(t *testing.T) {
	cases := []struct {
		center Center
		want   string
	}{
		{CenterUnspecified, "unspecified"},
		{CenterGeocentre, "geocentre"},
		{CenterBarycentre, "barycentre"},
		{CenterHeliocentre, "heliocentre"},
		{Center(200), "Center(200)"},
	}

	for _, c := range cases {
		if got := c.center.String(); got != c.want {
			t.Errorf("Center(%d).String() = %q, want %q", uint8(c.center), got, c.want)
		}
	}
}

func TestRequire(t *testing.T) {
	icrs := State{Frame: FrameICRS, Center: CenterGeocentre}
	gcrs := State{Frame: FrameGCRS, Center: CenterGeocentre}
	unlabelled := State{}

	cases := []struct {
		name    string
		state   State
		frame   Frame
		center  Center
		wantErr error
	}{
		{"matching frame and centre", icrs, FrameICRS, CenterGeocentre, nil},

		// The distinction the type exists to make: GCRS and ICRS differ by
		// frame bias, about 23 mas, and every provider used to hand back an
		// unlabelled State that was mathematically valid either way.
		{"wrong frame", gcrs, FrameICRS, CenterGeocentre, ErrWrongFrame},
		{"wrong centre", icrs, FrameICRS, CenterBarycentre, ErrWrongCenter},

		// An unspecified label asserts nothing, in either direction. A
		// provider that has not been taught to label its output says so
		// rather than claiming ICRS geocentric, and a caller that does not
		// care must not be forced to.
		{"unlabelled state passes", unlabelled, FrameICRS, CenterGeocentre, nil},
		{"caller requires nothing", gcrs, FrameUnspecified, CenterUnspecified, nil},
		{"caller requires frame only", gcrs, FrameGCRS, CenterUnspecified, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.state.Require(c.frame, c.center)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("Require(%s, %s) = %v, want %v", c.frame, c.center, err, c.wantErr)
			}
		})
	}
}

func TestRequireErrorNamesBothSides(t *testing.T) {
	// The message has to say what the state is and what was wanted; an error
	// reporting only "wrong frame" leaves the reader to guess which is which.
	err := State{Frame: FrameGCRS}.Require(FrameICRS, CenterUnspecified)

	got := fmt.Sprint(err)
	for _, want := range []string{"GCRS", "ICRS"} {
		if !strings.Contains(got, want) {
			t.Errorf("error %q does not name %q", got, want)
		}
	}
}
