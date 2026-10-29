package skybrightness

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/unit"
)

// uncertaintyEstimate is an estimate of three flat components on a 400-800 nm
// grid: zodiacal light twice as bright as airglow, and a starlight term that
// contributes nothing.
func uncertaintyEstimate(t *testing.T) *Estimate {
	t.Helper()

	grid, err := unit.NewSpectralGrid(400, 1, 401)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	flat := func(v float64) SpectralRadiance {
		s := NewSpectralRadiance(grid)
		for i := range s {
			s[i] = v
		}

		return s
	}

	return &Estimate{
		grid: grid,
		components: map[ComponentID]SpectralRadiance{
			Zodiacal:         flat(2e-9),
			AirglowContinuum: flat(1e-9),
			Starlight:        flat(0),
		},
	}
}

// TestTotalUncertaintyRefusesWhatIsUnknown: a component contributing light
// with no entry in the budget makes the total unknown, which is an error
// naming it, never 0 (#911). No component fills the budget yet, so every
// estimate reported an uncertainty of 0, a perfectly certain prediction.
func TestTotalUncertaintyRefusesWhatIsUnknown(t *testing.T) {
	t.Parallel()

	est := uncertaintyEstimate(t)

	// Nothing in the budget: both lit components are named, sorted, and the
	// dark one is not.
	total, err := est.TotalUncertainty()
	if !errors.Is(err, ErrUncertaintyUnknown) {
		t.Fatalf("TotalUncertainty = %v, %v; want ErrUncertaintyUnknown", total, err)
	}

	if msg := err.Error(); !strings.Contains(msg, "[airglow-continuum zodiacal]") {
		t.Errorf("the error %q does not name the two lit components", msg)
	}

	// One of the two covered: the other is still unknown.
	est.Uncertainty.Add(Uncertainty{Component: Zodiacal, Relative: 0.1})

	if _, err := est.TotalUncertainty(); !errors.Is(err, ErrUncertaintyUnknown) ||
		!strings.Contains(err.Error(), "airglow-continuum") || strings.Contains(err.Error(), "zodiacal") {
		t.Errorf("with zodiacal light covered: %v, want only airglow-continuum named", err)
	}
}

// TestTotalUncertaintyCombinesACoveredBudget: once every component giving
// light has an entry, the total is the budget's, weighted by each component's
// share of the integrated radiance. A component giving none needs no entry:
// its share is zero whatever its uncertainty.
func TestTotalUncertaintyCombinesACoveredBudget(t *testing.T) {
	t.Parallel()

	est := uncertaintyEstimate(t)
	est.Uncertainty.Add(Uncertainty{Component: Zodiacal, Relative: 0.1})
	est.Uncertainty.Add(Uncertainty{Component: AirglowContinuum, Relative: 0.5})

	total, err := est.TotalUncertainty()
	if err != nil {
		t.Fatalf("TotalUncertainty: %v", err)
	}

	// Shares 2/3 and 1/3, in quadrature.
	want := math.Hypot(2.0/3*0.1, 1.0/3*0.5)
	if math.Abs(total-want) > 1e-15 {
		t.Errorf("TotalUncertainty = %.17g, want %.17g", total, want)
	}
}
