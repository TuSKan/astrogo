package lsk

import "testing"

// TestLeapTableReadsMidnightAsTheDayItStarts pins the one piece of arithmetic
// between a kernel's DELTA_AT block and time's registry.
//
// The kernel records each step as a Julian Date at midnight UTC — a JD ending
// in .5, because JD rolls over at noon. A calendar conversion handed a value
// that has drifted an ULP low reads it as 23:59:59.999… of the *previous* day,
// and a leap second dated 1971-12-31 instead of 1972-01-01 is wrong by exactly
// the interval it exists to bound.
//
// Tested against leapTable directly rather than through RegisterLeapSeconds,
// because registration also enforces the ±1-second step rule — which would
// force this fixture to be a full 28-entry table just to probe a date
// conversion.
func TestLeapTableReadsMidnightAsTheDayItStarts(t *testing.T) {
	// Midnight UTC. 2441317.5 is the canonical JD of the start of UTC's
	// whole-second era and is quoted in naif0012.tls's own header.
	const (
		jd19720101 = 2441317.5
		jd20170101 = 2457754.5
	)

	cases := []struct {
		name                string
		jd                  float64
		wantY, wantM, wantD int
	}{
		{"exact midnight", jd19720101, 1972, 1, 1},
		{"an ULP low", jd19720101 - 1e-9, 1972, 1, 1},
		{"an ULP high", jd19720101 + 1e-9, 1972, 1, 1},
		{"a July step", 2441499.5, 1972, 7, 1},
		{"the last published step", jd20170101, 2017, 1, 1},
		{"the last published step, an ULP low", jd20170101 - 1e-9, 2017, 1, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := leapTable(&Reader{DeltaAt: []LeapData{{JD: tc.jd, N: 10}}})
			if err != nil {
				t.Fatalf("leapTable: %v", err)
			}

			if got[0].Year != tc.wantY || got[0].Month != tc.wantM || got[0].Day != tc.wantD {
				t.Errorf("JD %.9f became %04d-%02d-%02d, want %04d-%02d-%02d",
					tc.jd, got[0].Year, got[0].Month, got[0].Day, tc.wantY, tc.wantM, tc.wantD)
			}

			if got[0].DeltaAT != 10 {
				t.Errorf("DeltaAT = %g, want 10", got[0].DeltaAT)
			}
		})
	}
}

// TestLeapTableRejectsAKernelWithNoDeltaATBlock pins that a Reader carrying no
// leap seconds produces an error rather than an empty table that would then be
// installed and answer nothing.
func TestLeapTableRejectsAKernelWithNoDeltaATBlock(t *testing.T) {
	if _, err := leapTable(&Reader{}); err == nil {
		t.Error("a Reader with no DELTA_AT entries produced a table without error")
	}
}
