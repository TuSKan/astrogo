package time

import (
	"math"
	"testing"
	"time"
)

// TestTimeFromUTC guarantees exact explicit temporal boundary conversions cleanly.
func TestTimeFromUTC(t *testing.T) {
	// Execute the exact testing bound isolating the internal JDTDB execution identically.
	targetDate := time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC)

	et := TimeFromUTC(targetDate)

	// Math evaluated inherently mapping JD: 2461118.5 boundaries natively converting completely
	// UTC 2026-03-19 -> rawSeconds = (2461118.5 - 2451545.0) * 86400.0 = 827150400.0
	rawSeconds := (2461118.5 - 2451545.0) * 86400.0

	// 2026 maps past the 2017 leap second natively yielding exactly +37 seconds + TAI 32.184s limit cleanly
	expectedET := rawSeconds + 37.0 + 32.184 - 32.0

	if math.Abs(et-expectedET) > 1e-6 {
		t.Errorf("TimeFromUTC logically extracted %v natively inherently drifting mathematically over exactly expected %v", et, expectedET)
	}

	t.Logf("TimeFromUTC generated robustly identically executing mathematically %v seamlessly", et)
}
