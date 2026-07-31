package constants_test

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
)

func TestConstant_Quantity(t *testing.T) {
	c := constants.IAU.AstronomicalUnit

	q := c.Quantity()
	if q.Value != c.Value {
		t.Errorf("Quantity().Value = %v, want %v", q.Value, c.Value)
	}

	if q.Unit != c.Unit {
		t.Errorf("Quantity().Unit = %v, want %v", q.Unit, c.Unit)
	}

	km, err := q.In(unit.Kilometer)
	if err != nil {
		t.Fatalf("In(Kilometer): %v", err)
	}

	testutil.AssertRelNear(t, "AU in km", km.Value, 1.495978707e8, 1e-9)
}

func TestConstant_RelativeUncertainty(t *testing.T) {
	if got := constants.SI2019.SpeedOfLight.RelativeUncertainty(); got != 0 {
		t.Errorf("exact constant RelativeUncertainty() = %v, want 0", got)
	}

	g := constants.CODATA2022.GravitationalConstant
	if got, want := g.RelativeUncertainty(), 2.2e-5; math.Abs(got-want)/want > 1e-1 {
		t.Errorf("G RelativeUncertainty() = %v, want ~%v", got, want)
	}

	zero := constants.Constant{}
	if got := zero.RelativeUncertainty(); got != 0 {
		t.Errorf("zero-Value RelativeUncertainty() = %v, want 0 (no NaN)", got)
	}
}

func TestConstant_String(t *testing.T) {
	exact := constants.SI2019.SpeedOfLight.String()
	if !strings.Contains(exact, "exact") || !strings.Contains(exact, "m/s") {
		t.Errorf("SpeedOfLight.String() = %q, want it to mention exact and m/s", exact)
	}

	measured := constants.CODATA2022.GravitationalConstant.String()
	if !strings.Contains(measured, "±") {
		t.Errorf("GravitationalConstant.String() = %q, want it to contain the uncertainty symbol", measured)
	}

	dimless := constants.CODATA2022.FineStructureConstant.String()
	if strings.HasSuffix(dimless, " 1") || strings.HasSuffix(dimless, " ") {
		t.Errorf("FineStructureConstant.String() = %q, want no trailing dimensionless unit suffix", dimless)
	}
}

func TestSets_NameAndAllNonEmpty(t *testing.T) {
	for _, s := range constants.Sets() {
		if s.Name() == "" {
			t.Errorf("a set has an empty Name()")
		}

		if len(s.All()) == 0 {
			t.Errorf("set %q has an empty All()", s.Name())
		}
	}
}

func TestSets_Count(t *testing.T) {
	sets := constants.Sets()
	if len(sets) != 6 {
		t.Fatalf("len(Sets()) = %d, want 6", len(sets))
	}

	wantCounts := []int{3, 5, 5, 14, 2, 5} // SI2019, CODATA2022, CODATA2018, IAU2015, WGS84, Derived

	total := 0

	for i, s := range sets {
		n := len(s.All())
		total += n

		if n != wantCounts[i] {
			t.Errorf("set %d (%s): %d members, want %d", i, s.Name(), n, wantCounts[i])
		}
	}

	if total != 34 {
		t.Errorf("total constants across all sets = %d, want 34", total)
	}
}

// TestSets_AllCoversEveryConstantField walks each set struct's exported
// fields via reflection and confirms All() returns exactly its
// Constant-typed fields, in declaration order — so a field added to a set
// struct without also being added to All() is a build-time-invisible bug
// this test makes visible.
func TestSets_AllCoversEveryConstantField(t *testing.T) {
	constantType := reflect.TypeFor[constants.Constant]()

	check := func(name string, set constants.Set) {
		v := reflect.ValueOf(set)

		var fields []constants.Constant

		for i := range v.NumField() {
			if v.Type().Field(i).Type == constantType {
				if c, ok := v.Field(i).Interface().(constants.Constant); ok {
					fields = append(fields, c)
				}
			}
		}

		all := set.All()
		if len(all) != len(fields) {
			t.Fatalf("%s: All() has %d entries, struct has %d Constant fields", name, len(all), len(fields))
		}

		for i := range all {
			if !reflect.DeepEqual(all[i], fields[i]) {
				t.Errorf("%s: All()[%d] does not match struct field %d in declaration order", name, i, i)
			}
		}
	}

	check("SI2019", constants.SI2019)
	check("CODATA2022", constants.CODATA2022)
	check("CODATA2018", constants.CODATA2018)
	check("IAU2015", constants.IAU2015)
	check("WGS84", constants.WGS84)
	check("Derived", constants.Derived)
}

func TestConstants_MetadataComplete(t *testing.T) {
	for _, s := range constants.Sets() {
		for _, c := range s.All() {
			if c.Name == "" {
				t.Errorf("%s: a constant has an empty Name (Symbol=%q)", s.Name(), c.Symbol)
			}

			if c.Symbol == "" {
				t.Errorf("%s: a constant has an empty Symbol (Name=%q)", s.Name(), c.Name)
			}

			if c.Reference == "" {
				t.Errorf("%s: %s has an empty Reference", s.Name(), c.Symbol)
			}
		}
	}
}

// TestConstants_ExactImpliesZeroUncertainty is one-directional by design:
// Exact ⇒ Uncertainty == 0, but Uncertainty == 0 does NOT imply Exact —
// the IAU WGCCRE body radii are !Exact with Uncertainty == 0 (their
// source publishes no uncertainty figure).
func TestConstants_ExactImpliesZeroUncertainty(t *testing.T) {
	for _, s := range constants.Sets() {
		for _, c := range s.All() {
			if c.Exact && c.Uncertainty != 0 {
				t.Errorf("%s: %s is Exact but Uncertainty = %v, want 0", s.Name(), c.Symbol, c.Uncertainty)
			}

			if c.Uncertainty > 0 && c.Exact {
				t.Errorf("%s: %s has Uncertainty > 0 but Exact = true", s.Name(), c.Symbol)
			}

			if c.Uncertainty < 0 {
				t.Errorf("%s: %s has negative Uncertainty %v", s.Name(), c.Symbol, c.Uncertainty)
			}

			if math.IsNaN(c.Value) || math.IsInf(c.Value, 0) {
				t.Errorf("%s: %s has non-finite Value %v", s.Name(), c.Symbol, c.Value)
			}

			if math.IsNaN(c.Uncertainty) || math.IsInf(c.Uncertainty, 0) {
				t.Errorf("%s: %s has non-finite Uncertainty %v", s.Name(), c.Symbol, c.Uncertainty)
			}
		}
	}
}

func TestConstants_ValuesFiniteAndPositive(t *testing.T) {
	for _, s := range constants.Sets() {
		for _, c := range s.All() {
			if !(c.Value > 0) {
				t.Errorf("%s: %s has Value = %v, want > 0", s.Name(), c.Symbol, c.Value)
			}
		}
	}
}

func TestConstants_UnitsAreSIBaseScaled(t *testing.T) {
	for _, s := range constants.Sets() {
		for _, c := range s.All() {
			if c.Unit.ScaleFactor != 1 {
				t.Errorf("%s: %s has Unit.ScaleFactor = %v, want 1 (Value must be the SI base-unit value)",
					s.Name(), c.Symbol, c.Unit.ScaleFactor)
			}
		}
	}
}

func TestConstants_NoDuplicateNameOrSymbolWithinSet(t *testing.T) {
	for _, s := range constants.Sets() {
		seenName := make(map[string]bool)
		seenSymbol := make(map[string]bool)

		for _, c := range s.All() {
			if seenName[c.Name] {
				t.Errorf("%s: duplicate Name %q", s.Name(), c.Name)
			}

			seenName[c.Name] = true

			if seenSymbol[c.Symbol] {
				t.Errorf("%s: duplicate Symbol %q", s.Name(), c.Symbol)
			}

			seenSymbol[c.Symbol] = true
		}
	}
}

// TestConstants_RelativeUncertaintyIsSmall catches an exponent typo in an
// uncertainty — the worst real case among this package's constants is
// IAU2015's Pluto equatorial radius at a relative uncertainty of
// ~1.35e-3 (1.6 km on 1188.3 km — Pluto's shape is the least precisely
// determined body in the WGCCRE table), which is why the gate sits at
// 5e-3 rather than tighter: still an order of magnitude of headroom
// above every real value here, so a genuine exponent typo (10x the
// legitimate uncertainty) still trips it.
func TestConstants_RelativeUncertaintyIsSmall(t *testing.T) {
	for _, s := range constants.Sets() {
		for _, c := range s.All() {
			if c.Exact || c.Uncertainty == 0 {
				continue
			}

			if rel := c.RelativeUncertainty(); rel >= 5e-3 {
				t.Errorf("%s: %s has implausibly large relative uncertainty %v", s.Name(), c.Symbol, rel)
			}
		}
	}
}
