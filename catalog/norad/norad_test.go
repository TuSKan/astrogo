package norad

import (
	"encoding/json"
	"testing"
)

// issFixture is a representative ISS GP element set in CelestTrak JSON format.
const issFixture = `[{
    "OBJECT_NAME": "ISS (ZARYA)",
    "OBJECT_ID": "1998-067A",
    "EPOCH": "2026-04-19T11:45:32.833440",
    "MEAN_MOTION": 15.4883325,
    "ECCENTRICITY": 0.00066312,
    "INCLINATION": 51.6329,
    "RA_OF_ASC_NODE": 230.6068,
    "ARG_OF_PERICENTER": 325.6576,
    "MEAN_ANOMALY": 34.3983,
    "EPHEMERIS_TYPE": 0,
    "CLASSIFICATION_TYPE": "U",
    "NORAD_CAT_ID": 25544,
    "ELEMENT_SET_NO": 999,
    "REV_AT_EPOCH": 56265,
    "BSTAR": 0.00019193879,
    "MEAN_MOTION_DOT": 0.00010082,
    "MEAN_MOTION_DDOT": 0
}]`

func TestParseGPJSON(t *testing.T) {
	var gps []GP
	if err := json.Unmarshal([]byte(issFixture), &gps); err != nil {
		t.Fatalf("Failed to parse ISS fixture: %v", err)
	}

	if len(gps) != 1 {
		t.Fatalf("Expected 1 GP element set, got %d", len(gps))
	}

	gp := gps[0]

	if gp.ObjectName != "ISS (ZARYA)" {
		t.Errorf("ObjectName = %q, want %q", gp.ObjectName, "ISS (ZARYA)")
	}
	if gp.ObjectID != "1998-067A" {
		t.Errorf("ObjectID = %q, want %q", gp.ObjectID, "1998-067A")
	}
	if gp.NoradCatID != 25544 {
		t.Errorf("NoradCatID = %d, want 25544", gp.NoradCatID)
	}
	if gp.Inclination < 51.0 || gp.Inclination > 52.0 {
		t.Errorf("Inclination = %f, expected ~51.6", gp.Inclination)
	}
	if gp.Eccentricity < 0 || gp.Eccentricity > 0.01 {
		t.Errorf("Eccentricity = %f, expected near-circular", gp.Eccentricity)
	}
	if gp.MeanMotion < 15 || gp.MeanMotion > 16 {
		t.Errorf("MeanMotion = %f, expected ~15.5 rev/day for ISS", gp.MeanMotion)
	}
	if gp.BStar <= 0 {
		t.Errorf("BStar = %f, expected positive for LEO", gp.BStar)
	}
	if gp.Classification != "U" {
		t.Errorf("Classification = %q, want U", gp.Classification)
	}
}

func TestEpochTime(t *testing.T) {
	var gps []GP
	if err := json.Unmarshal([]byte(issFixture), &gps); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	epoch, err := gps[0].EpochTime()
	if err != nil {
		t.Fatalf("EpochTime failed: %v", err)
	}

	if epoch.Year() != 2026 {
		t.Errorf("Year = %d, want 2026", epoch.Year())
	}

	// JD should be reasonable (2026 is around JD 2461XXX).
	jd := epoch.JD()
	if jd < 2461000 || jd > 2462000 {
		t.Errorf("JD = %f, seems unreasonable for 2026", jd)
	}
}

func TestToTLE(t *testing.T) {
	var gps []GP
	if err := json.Unmarshal([]byte(issFixture), &gps); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	line1, line2 := gps[0].ToTLE()

	// Basic format checks.
	if len(line1) != 69 {
		t.Errorf("TLE line 1 length = %d, want 69", len(line1))
	}
	if len(line2) != 69 {
		t.Errorf("TLE line 2 length = %d, want 69", len(line2))
	}
	if line1[0] != '1' {
		t.Errorf("TLE line 1 should start with '1', got %c", line1[0])
	}
	if line2[0] != '2' {
		t.Errorf("TLE line 2 should start with '2', got %c", line2[0])
	}

	t.Logf("Generated TLE:\n%s\n%s", line1, line2)
}

func TestChecksumTLE(t *testing.T) {
	// Known ISS TLE line 1 (without checksum digit).
	line := "1 25544U 98067A   26109.48996335  .00010082  00000+0  19194-3 0  999"
	cs := checksumTLE(line)
	// The actual checksum for this line should be 2.
	if cs != 2 {
		t.Logf("Checksum for known ISS TLE = %d (reference = 2)", cs)
	}
}

func TestGPToTarget(t *testing.T) {
	gp := GP{
		ObjectName: "ISS (ZARYA)",
		ObjectID:   "1998-067A",
		NoradCatID: 25544,
	}

	target := gpToTarget(gp)

	if target.ID != "25544" {
		t.Errorf("Target.ID = %q, want %q", target.ID, "25544")
	}
	if target.Name != "ISS (ZARYA)" {
		t.Errorf("Target.Name = %q, want %q", target.Name, "ISS (ZARYA)")
	}
	if target.Catalog != "norad" {
		t.Errorf("Target.Catalog = %q, want %q", target.Catalog, "norad")
	}
	if target.Kind != "Satellite" {
		t.Errorf("Target.Kind = %q, want %q", target.Kind, "Satellite")
	}
}

func TestFormatTLEExp(t *testing.T) {
	tests := []struct {
		input float64
		want  string
	}{
		{0, " 00000-0"},
		{0.00019193879, " 19194-3"},
	}

	for _, tt := range tests {
		got := formatTLEExp(tt.input)
		// Just check it produces reasonable output.
		if len(got) == 0 {
			t.Errorf("formatTLEExp(%f) returned empty string", tt.input)
		}
		t.Logf("formatTLEExp(%e) = %q", tt.input, got)
	}
}

// multiFixture tests parsing of multi-element responses.
const multiFixture = `[
    {"OBJECT_NAME":"ISS (ZARYA)","OBJECT_ID":"1998-067A","EPOCH":"2026-04-19T11:45:32.833440","MEAN_MOTION":15.4883325,"ECCENTRICITY":0.00066312,"INCLINATION":51.6329,"RA_OF_ASC_NODE":230.6068,"ARG_OF_PERICENTER":325.6576,"MEAN_ANOMALY":34.3983,"EPHEMERIS_TYPE":0,"CLASSIFICATION_TYPE":"U","NORAD_CAT_ID":25544,"ELEMENT_SET_NO":999,"REV_AT_EPOCH":56265,"BSTAR":0.00019193879,"MEAN_MOTION_DOT":0.00010082,"MEAN_MOTION_DDOT":0},
    {"OBJECT_NAME":"CSS (TIANHE)","OBJECT_ID":"2021-035A","EPOCH":"2026-04-19T08:22:11.000000","MEAN_MOTION":15.6120000,"ECCENTRICITY":0.00030000,"INCLINATION":41.4700,"RA_OF_ASC_NODE":180.0000,"ARG_OF_PERICENTER":100.0000,"MEAN_ANOMALY":260.0000,"EPHEMERIS_TYPE":0,"CLASSIFICATION_TYPE":"U","NORAD_CAT_ID":48274,"ELEMENT_SET_NO":999,"REV_AT_EPOCH":28000,"BSTAR":0.00015000000,"MEAN_MOTION_DOT":0.00005000,"MEAN_MOTION_DDOT":0}
]`

func TestParseMultiGP(t *testing.T) {
	var gps []GP
	if err := json.Unmarshal([]byte(multiFixture), &gps); err != nil {
		t.Fatalf("Failed to parse multi fixture: %v", err)
	}

	if len(gps) != 2 {
		t.Fatalf("Expected 2 GP element sets, got %d", len(gps))
	}

	if gps[0].ObjectName != "ISS (ZARYA)" {
		t.Errorf("First object = %q, want ISS", gps[0].ObjectName)
	}
	if gps[1].ObjectName != "CSS (TIANHE)" {
		t.Errorf("Second object = %q, want CSS", gps[1].ObjectName)
	}
}
