//go:build integration

package magnitude_test

import "testing"

// TestFinkResidualsAllNullTellsAFailedComputationFromAMissingColumn pins the
// line #700 draws. Every record carrying residuals_shg1g2 with a null value is
// FINK reporting that it computed none, a degraded service, and the live
// tests skip. A column that is absent, or one value that is present, is not:
// an absent column is FINK's schema changing under the test, which the
// control query must still get to call wrong data.
func TestFinkResidualsAllNullTellsAFailedComputationFromAMissingColumn(t *testing.T) {
	t.Parallel()

	null := map[string]any{"residuals_shg1g2": nil}
	value := map[string]any{"residuals_shg1g2": 0.12}
	absent := map[string]any{"i:jd": 2460000.5}

	for _, c := range []struct {
		name    string
		records []map[string]any
		want    bool
	}{
		{"every residual null", []map[string]any{null, null, null}, true},
		{"one residual present", []map[string]any{null, value, null}, false},
		{"the column absent", []map[string]any{absent, absent}, false},
		{"absent from one record", []map[string]any{null, absent}, false},
		{"no records", nil, false},
	} {
		if got := finkResidualsAllNull(c.records); got != c.want {
			t.Errorf("%s: finkResidualsAllNull = %v, want %v", c.name, got, c.want)
		}
	}
}
