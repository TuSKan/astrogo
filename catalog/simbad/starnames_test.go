package simbad

import "testing"

func TestFriendlyName(t *testing.T) {
	tests := []struct {
		mainID   string
		wantName string
		wantOK   bool
	}{
		{"* alf CMa", "Sirius", true},
		{"* bet Ori", "Rigel", true},
		{"* alf Boo", "Arcturus", true},
		// Component-letter stripping: "alf Cen A" -> "alf Cen" + " A".
		{"* alf Cen A", "Rigil Kentaurus A", true},
		{"* alf Cen B", "Rigil Kentaurus B", true},
		// Doubled Bayer-letter components looked up as explicit entries,
		// not via the generic single-letter-suffix stripping.
		{"* alf01 Cru", "Acrux", true},
		{"* alf02 Cru", "Acrux", true},
		{"* gam02 Vel", "Regor", true},
		// Unrecognized designations fall back to mainID unchanged.
		{"* zet Aqr", "* zet Aqr", false},
		{"NAME LMC", "NAME LMC", false},
		{"SN 2009jb", "SN 2009jb", false},
	}

	for _, tt := range tests {
		t.Run(tt.mainID, func(t *testing.T) {
			gotName, gotOK := friendlyName(tt.mainID)
			if gotName != tt.wantName || gotOK != tt.wantOK {
				t.Errorf("friendlyName(%q) = (%q, %v), want (%q, %v)", tt.mainID, gotName, gotOK, tt.wantName, tt.wantOK)
			}
		})
	}
}

// TestFriendlyName_AllEntriesNonEmpty guards against a typo leaving a
// starNames value empty, which friendlyName would otherwise silently
// accept as a "found" result.
func TestFriendlyName_AllEntriesNonEmpty(t *testing.T) {
	for designation, name := range starNames {
		if name == "" {
			t.Errorf("starNames[%q] is empty", designation)
		}
	}
}
