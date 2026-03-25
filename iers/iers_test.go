package iers

import (
	"strings"
	"testing"
)

func TestParseIERSExact(t *testing.T) {
	// Exact dummy mock simulating finals2000A.data
	mockData := `23 1 1 59945.00 I -0.016335 0.000085  0.222287 0.000080  I-0.0465243 0.0000103  1.3093 0.0051
23 1 2 59946.00 I -0.015243 0.000086  0.224151 0.000080  I-0.0476483 0.0000104  1.3201 0.0051
Short Line Abort
23 1 3 59947.00 I -0.014234 0.000087  0.225915 0.000082  I-0.0487007 0.0000106  1.3340 0.0051`

	r := strings.NewReader(mockData)
	res, err := ParseIERS(r)

	if err != nil {
		t.Fatalf("unexpected failure resolving parse streams: %v", err)
	}
	
	// Inject the isolated structure natively so the global accessor works correctly
	globalOffsets = res


	if len(res) != 3 {
		t.Fatalf("expected 3 valid structurally bound offsets, got %d", len(res))
	}

	// Evaluates standard boundaries directly returning true float conversions.
	if offset := GetOffset(59945); offset != -0.0465243 {
		t.Errorf("expected offset -0.0465243 tracking MJD 59945, obtained %f", offset)
	}

	// Validate generic invalid boundary skips seamlessly parsing 0.0 natively.
	if voidOffset := GetOffset(99999); voidOffset != 0.0 {
		t.Errorf("expected missing offset struct returning 0.0 inherently, obtained %f", voidOffset)
	}
}
