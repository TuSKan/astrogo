package starlight

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

// nonFinite are the radiances neither reader may keep, as ParseFloat spells
// them.
var nonFinite = []string{"+Inf", "Inf", "-Inf", "NaN"}

// orderOneMap is a plausible order-1 map, its 48 radiances spanning a factor
// of 48, with pixel 5's radiance written as radiance.
func orderOneMap(radiance string) *bytes.Buffer {
	var b bytes.Buffer

	b.WriteString(GaiaBuild{Order: 1, Bands: []GaiaBand{GaiaJohnsonV()}}.Header())

	for pixel := range 48 {
		v := fmt.Sprintf("%.6e", 1e-10*float64(1+pixel))
		if pixel == 5 {
			v = radiance
		}

		fmt.Fprintf(&b, "%d %s\n", pixel, v)
	}

	return &b
}

// Load refuses a map with a non-finite radiance as malformed. It refused NaN
// and negative values before, but +Inf passed, and plausibleSky passed the map
// too, its contrast being infinite (#1030).
func TestLoadRefusesANonFiniteRadiance(t *testing.T) {
	t.Parallel()

	if _, err := Load(orderOneMap("6.000000e-10"), ICRS); err != nil {
		t.Fatalf("Load of the map with a finite pixel 5: %v", err)
	}

	for _, radiance := range nonFinite {
		if _, err := Load(orderOneMap(radiance), ICRS); !errors.Is(err, ErrMapFormat) {
			t.Errorf("Load with pixel 5 = %s: err %v, want ErrMapFormat", radiance, err)
		}
	}
}

// The build cache skips a non-finite radiance like any value it cannot use,
// and keeps reading the lines around it. Its check, v <= 0, let NaN and +Inf
// through (#1030).
func TestParseCacheSkipsANonFiniteRadiance(t *testing.T) {
	t.Parallel()

	var cache strings.Builder

	cache.WriteString("# bands: V\n")

	for i, radiance := range nonFinite {
		fmt.Fprintf(&cache, "%d %s 3\n", i, radiance)
	}

	fmt.Fprintf(&cache, "%d 1.5e-10 4\n", len(nonFinite))

	values := make([]float64, 8)
	counts := make([]int64, 8)

	if err := parseCache(strings.NewReader(cache.String()), values, counts); err != nil {
		t.Fatalf("parseCache: %v", err)
	}

	for i, radiance := range nonFinite {
		if values[i] != 0 || counts[i] != 0 {
			t.Errorf("pixel %d, radiance %s: kept as %v with count %d, want skipped", i, radiance, values[i], counts[i])
		}
	}

	if got := values[len(nonFinite)]; math.Abs(got-1.5e-10) > 1e-22 || counts[len(nonFinite)] != 4 {
		t.Errorf("the finite line after them: %v with count %d, want 1.5e-10 with count 4", got, counts[len(nonFinite)])
	}
}
