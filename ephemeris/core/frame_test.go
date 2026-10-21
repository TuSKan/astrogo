package core

import (
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
