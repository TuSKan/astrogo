package plan

import (
	"math"
	"strings"
	"testing"
)

// threshold is the value of the quantity set by at where visible turns from
// false to true, found by bisection between lo, invisible, and hi, visible.
func threshold(t *testing.T, lo, hi float64, visible func(x float64) bool) float64 {
	t.Helper()

	if visible(lo) || !visible(hi) {
		t.Fatalf("no threshold in [%g, %g]: visible(lo) = %v, visible(hi) = %v", lo, hi, visible(lo), visible(hi))
	}

	for range 60 {
		mid := (lo + hi) / 2
		if visible(mid) {
			hi = mid
		} else {
			lo = mid
		}
	}

	return hi
}

// lineTest checks that a criterion's line passes through each (x, y) point
// within tol, x the value set by setX and y the threshold of the value set by
// setY.
func lineTest(t *testing.T, name string, xs, ys []float64, tol float64,
	criterion func(*CrescentParams) bool, setX, setY func(*CrescentParams, float64),
) {
	t.Helper()

	for i, x := range xs {
		got := threshold(t, -5, 40, func(y float64) bool {
			var p CrescentParams

			setX(&p, x)
			setY(&p, y)

			return criterion(&p)
		})

		if math.Abs(got-ys[i]) > tol {
			t.Errorf("%s at %g: line at %.3f, the source has %.3f (tolerance %g)", name, x, got, ys[i], tol)
		}
	}
}

func setDAZ(p *CrescentParams, v float64)  { p.DAZ = v }
func setMAlt(p *CrescentParams, v float64) { p.MAlt = v }
func setArcV(p *CrescentParams, v float64) { p.ArcV = v }
func setW(p *CrescentParams, v float64)    { p.W = v }

// Fotheringham (1910), p. 531: "Minimum Altitude = 12°·0 − 0°·008 Z²". The
// linear 12.0 − 0.008·DAZ this package had asked 11.84° at DAZ 20° where
// Fotheringham asks 8.8° (#503).
func TestFotheringhamIsHisFormula(t *testing.T) {
	t.Parallel()

	daz := []float64{0, 5, 10, 15, 20, 23}
	want := make([]float64, len(daz))

	for i, z := range daz {
		want[i] = 12.0 - 0.008*z*z
	}

	lineTest(t, "Fotheringham", daz, want, 1e-9, (*CrescentParams).Fotheringham, setDAZ, setMAlt)
}

// Maunder (1911), p. 359, as Yallop (1997) Table 1 reproduces it. The
// −0.005·DAZ this package had put the line 0.9° high at DAZ 20° (#503).
func TestMaunderReproducesHisTable(t *testing.T) {
	t.Parallel()

	lineTest(t, "Maunder",
		[]float64{0, 5, 10, 15, 20},
		[]float64{11.0, 10.5, 9.5, 8.0, 6.0},
		1e-9, (*CrescentParams).Maunder, setDAZ, setMAlt)
}

// Ilyas (1988), Fig. 5: the curve levels off at about 4° beyond DAZ 35°. The
// cubic this package had fell to 1.9° at DAZ 40° and climbed back to 6.7° at
// 60° (#503).
func TestIlyas1988FollowsHisCurve(t *testing.T) {
	t.Parallel()

	lineTest(t, "Ilyas 1988",
		[]float64{0, 10, 20, 30, 40, 60, 75},
		[]float64{10.29, 9.21, 6.42, 4.51, 4.23, 4.12, 4.12},
		1e-9, (*CrescentParams).Ilyas1988, setDAZ, setMAlt)
}

// Krauss (2012), Table 13, the Athenian column.
func TestKraussAthenianReproducesTable13(t *testing.T) {
	t.Parallel()

	lineTest(t, "Krauss",
		[]float64{0, 5, 10, 15, 20, 22, 30},
		[]float64{10.6, 10.5, 9.95, 9.0, 7.6, 7.0, 7.0},
		1e-9, (*CrescentParams).KraussAthenian, setDAZ, setMAlt)
}

// Yallop (1997), Table 3: Bruin's curves, which Yallop's least-squares cubic
// (eq. 3.3) fits to 0.18°, its largest residual at W = 0.5′; the W = 0.3′
// point is Yallop's own extrapolation.
func TestBruinFitsHisCurves(t *testing.T) {
	t.Parallel()

	lineTest(t, "Bruin",
		[]float64{0.3, 0.5, 0.7, 1, 2, 3},
		[]float64{10.0, 8.4, 7.5, 6.4, 4.7, 4.3},
		0.18, (*CrescentParams).Bruin, setW, setArcV)
}

// Alrefay et al. (2018), eqs. 8 and 9.
func TestAlrefayIsTheirEquations(t *testing.T) {
	t.Parallel()

	w := []float64{0.2, 0.5, 1.0, 1.5}
	naked, aided := make([]float64, len(w)), make([]float64, len(w))

	for i, x := range w {
		naked[i] = 9.34 - 4.51*x + 3.3*x*x - 1.01*x*x*x
		aided[i] = 7.83 - 4.35*x + 3.22*x*x - 1.02*x*x*x
	}

	lineTest(t, "Alrefay naked eye", w, naked, 1e-9, (*CrescentParams).AlrefayNakedEye, setW, setArcV)
	lineTest(t, "Alrefay optical aid", w, aided, 1e-9, (*CrescentParams).AlrefayOpticalAid, setW, setArcV)
}

// Caldwell & Laney (2001), Table 1, both lines, and halfway between two
// entries.
func TestCaldwellReproducesTable1(t *testing.T) {
	t.Parallel()

	lineTest(t, "SAAO naked eye",
		[]float64{0, 5, 10, 10.25, 15, 21, 30},
		[]float64{8.19, 7.77, 6.84, 6.785, 5.67, 4.33, 4.33},
		1e-9, (*CrescentParams).CaldwellNakedEye, setDAZ, setMAlt)
	lineTest(t, "SAAO optical",
		[]float64{0, 5, 10, 15, 21},
		[]float64{6.29, 5.87, 4.94, 3.77, 2.43},
		1e-9, (*CrescentParams).CaldwellOptical, setDAZ, setMAlt)
}

// Qureshi (2010), Table 5: three observations, from their ARCV and width to
// his s-value and Yallop's q. Eq. 5 as printed, which this package followed,
// put all three in zone A (#503).
func TestQureshiReproducesHisTable5(t *testing.T) {
	t.Parallel()

	for _, o := range []struct {
		no       int
		arcv     float64 // degrees
		widthSec float64 // arcseconds
		q, s     float64
		zone     string
	}{
		{220, 6.03, 23.3, -0.35, -0.26, "E"},
		{257, 7.61, 8.73, -0.33, -0.21, "E"},
		{79, 10.7, 24.7, 0.135, 0.215, "A"},
	} {
		p := CrescentParams{ArcV: o.arcv, W: o.widthSec / 60}

		z := p.Qureshi()
		if math.Abs(z.Value-o.s) > 0.006 || z.Code != o.zone {
			t.Errorf("obs. %d: s = %.4f, zone %s; Qureshi's Table 5 has %.3f, zone %s", o.no, z.Value, z.Code, o.s, o.zone)
		}

		if q := p.Yallop().Value; math.Abs(q-o.q) > 0.006 {
			t.Errorf("obs. %d: q = %.4f; Qureshi's Table 5 has %.3f", o.no, q, o.q)
		}
	}
}

func TestElongationLimits(t *testing.T) {
	t.Parallel()

	setArcL := func(p *CrescentParams, v float64) { p.ArcL = v }
	none := func(*CrescentParams, float64) {}

	for _, c := range []struct {
		name      string
		criterion func(*CrescentParams) bool
		limit     float64
	}{
		{"Danjon", (*CrescentParams).Danjon, 7.0},
		{"Fatoohi 1998", (*CrescentParams).Fatoohi1998, 7.5},
		{"Ilyas 1983", (*CrescentParams).Ilyas1983, 10.5},
	} {
		lineTest(t, c.name, []float64{0}, []float64{c.limit}, 1e-9, c.criterion, none, setArcL)
	}
}

func TestCalendricalCriteria(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name      string
		criterion func(*CrescentParams) bool
		p         CrescentParams
		want      bool
	}{
		{"MABIMS 2021", (*CrescentParams).MABIMS2021, CrescentParams{MAlt: 3, ArcL: 6.4}, true},
		{"MABIMS 2021 low", (*CrescentParams).MABIMS2021, CrescentParams{MAlt: 2.9, ArcL: 10}, false},
		{"MABIMS 2021 close", (*CrescentParams).MABIMS2021, CrescentParams{MAlt: 10, ArcL: 6.3}, false},
		{"MABIMS 1995 elongation", (*CrescentParams).MABIMS1995, CrescentParams{MAlt: 2.5, ArcL: 3.5, Age: 6}, true},
		{"MABIMS 1995 age", (*CrescentParams).MABIMS1995, CrescentParams{MAlt: 2.5, ArcL: 2.5, Age: 8}, true},
		{"MABIMS 1995 neither", (*CrescentParams).MABIMS1995, CrescentParams{MAlt: 2.5, ArcL: 2.5, Age: 7.9}, false},
		{"MABIMS 1995 too low", (*CrescentParams).MABIMS1995, CrescentParams{MAlt: 1.9, ArcL: 10, Age: 30}, false},
		{"Istanbul 2016", (*CrescentParams).Istanbul2016, CrescentParams{MAlt: 5, ArcL: 8}, true},
		{"Istanbul 2016 low", (*CrescentParams).Istanbul2016, CrescentParams{MAlt: 4.9, ArcL: 12}, false},
	} {
		p := c.p
		if got := c.criterion(&p); got != c.want {
			t.Errorf("%s(%+v) = %v, want %v", c.name, c.p, got, c.want)
		}
	}
}

func TestInterpolationHoldsTheEnds(t *testing.T) {
	t.Parallel()

	ys := []float64{3, 2, 1}

	for _, c := range []struct{ x, want float64 }{{-1, 3}, {0, 3}, {0.5, 2.5}, {2, 1}, {9, 1}} {
		if got := interpolateUniform(ys, 1, c.x); got != c.want {
			t.Errorf("interpolateUniform(%g) = %g, want %g", c.x, got, c.want)
		}

		if got := interpolateTable([]float64{0, 1, 2}, ys, c.x); got != c.want {
			t.Errorf("interpolateTable(%g) = %g, want %g", c.x, got, c.want)
		}
	}
}

func TestYallop(t *testing.T) {
	t.Parallel()

	// At W=0: q = (ArcV - 11.8371) / 10
	// Zone A: q > +0.216  → ArcV > 13.9971
	// Zone B: +0.216 >= q > -0.014  → 13.9971 >= ArcV > 11.6971
	// Zone C: -0.014 >= q > -0.160  → 11.6971 >= ArcV > 10.2371
	// Zone D: -0.160 >= q > -0.232  → 10.2371 >= ArcV > 9.5171
	// Zone E: -0.232 >= q > -0.293  → 9.5171 >= ArcV > 8.9071
	// Zone F: q <= -0.293  → ArcV <= 8.9071
	tests := []struct {
		name     string
		wantCode string
		p        CrescentParams
	}{
		{"zone A easily visible", "A", CrescentParams{ArcV: 15, W: 0}},
		{"zone B", "B", CrescentParams{ArcV: 13.0, W: 0}},
		{"zone C", "C", CrescentParams{ArcV: 11.0, W: 0}},
		{"zone D", "D", CrescentParams{ArcV: 10.0, W: 0}},
		{"zone E", "E", CrescentParams{ArcV: 9.2, W: 0}},
		{"zone F below Danjon", "F", CrescentParams{ArcV: 2, W: 0}},
	}
	for _, tt := range tests {
		got := tt.p.Yallop()
		if got.Code != tt.wantCode {
			t.Errorf("%s: Yallop() code = %q, want %q (q=%.6f)", tt.name, got.Code, tt.wantCode, got.Value)
		}

		if got.Params != tt.p {
			t.Errorf("%s: zone carries %+v, want the parameters it read, %+v", tt.name, got.Params, tt.p)
		}
	}
}

func TestOdeh(t *testing.T) {
	t.Parallel()

	// At W=0: V = ArcV - 7.1651
	// Naked Eye:      V >= 5.65  → ArcV >= 12.8151
	// Optical/Naked:  5.65 > V >= 2.0  → 12.8151 > ArcV >= 9.1651
	// Optical Only:   2.0 > V >= -0.96  → 9.1651 > ArcV >= 6.2051
	// Not Visible:    V < -0.96  → ArcV < 6.2051
	tests := []struct {
		name     string
		wantCode string
		p        CrescentParams
	}{
		{"naked eye", "Naked Eye", CrescentParams{ArcV: 15, W: 0}},
		{"optical/naked", "Optical/Naked", CrescentParams{ArcV: 10, W: 0}},
		{"just below naked eye", "Optical/Naked", CrescentParams{ArcV: 12.81, W: 0}},
		{"optical only", "Optical Only", CrescentParams{ArcV: 7, W: 0}},
		{"not visible", "Not Visible", CrescentParams{ArcV: 2, W: 0}},
	}
	for _, tt := range tests {
		got := tt.p.Odeh()
		if got.Code != tt.wantCode {
			t.Errorf("%s: Odeh() code = %q, want %q (V=%.6f)", tt.name, got.Code, tt.wantCode, got.Value)
		}
	}
}

func TestCrescentZoneString(t *testing.T) {
	t.Parallel()

	z := CrescentZone{Code: "A", Label: "Easily visible", Value: 0.3456}

	s := z.String()
	if s != "A: Easily visible (value=0.3456)" {
		t.Errorf("CrescentZone.String() = %q", s)
	}
}

func TestCrescentResultStringNamesEveryCriterion(t *testing.T) {
	t.Parallel()

	s := CrescentResult{}.String()

	for _, want := range []string{
		"Fotheringham", "Maunder", "Ilyas (1988)", "Krauss", "Danjon", "Fatoohi", "Ilyas (1983)",
		"MABIMS (1995)", "MABIMS (2021)", "Istanbul", "Bruin", "Alrefay naked", "Alrefay aided",
		"SAAO naked", "SAAO aided", "Yallop", "Odeh", "Qureshi",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("String() omits %q:\n%s", want, s)
		}
	}

	// One verdict visible, the rest not, each on its own line.
	s = CrescentResult{Danjon: CrescentVerdict{Visible: true}}.String()

	for line := range strings.Lines(s) {
		visible := strings.Contains(line, " visible ") && !strings.Contains(line, "not visible")

		switch {
		case strings.Contains(line, "Danjon") && !visible:
			t.Errorf("the visible Danjon verdict reads %q", line)
		case !strings.Contains(line, "Danjon") && visible:
			t.Errorf("a verdict that is not visible reads %q", line)
		}
	}
}
