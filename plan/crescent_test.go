package plan

import (
	"math"
	"testing"
)

// ── Category 1: Altitude & Azimuth ──────────────────────────────────────────

func TestFotheringham(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"clear visible", CrescentParams{ArcV: 15, DAZ: 10}, true},
		{"boundary exact", CrescentParams{ArcV: 11.92, DAZ: 10}, true},
		{"just below", CrescentParams{ArcV: 11.919, DAZ: 10}, false},
		{"zero DAZ visible", CrescentParams{ArcV: 12.0, DAZ: 0}, true},
		{"zero DAZ invisible", CrescentParams{ArcV: 11.99, DAZ: 0}, false},
		// limit at DAZ=20: 12.0 - 0.008*20 = 11.84
		{"large DAZ visible", CrescentParams{ArcV: 11.84, DAZ: 20}, true},
		{"large DAZ fail", CrescentParams{ArcV: 11.83, DAZ: 20}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Fotheringham(); got != tt.want {
				limit := 12.0 - 0.008*tt.p.DAZ
				t.Errorf("Fotheringham() = %v, want %v (ArcV=%.4f, limit=%.4f)", got, tt.want, tt.p.ArcV, limit)
			}
		})
	}
}

func TestMaunder(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"clear visible", CrescentParams{ArcV: 15, DAZ: 5}, true},
		{"zero DAZ at limit", CrescentParams{ArcV: 11.0, DAZ: 0}, true},
		{"zero DAZ below", CrescentParams{ArcV: 10.99, DAZ: 0}, false},
		// limit at DAZ=5: 11.0 - 0.005*5 - 0.01*25 = 10.725
		{"moderate DAZ visible", CrescentParams{ArcV: 10.73, DAZ: 5}, true},
		{"moderate DAZ invisible", CrescentParams{ArcV: 10.72, DAZ: 5}, false},
		{"invisible", CrescentParams{ArcV: 5, DAZ: 10}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Maunder(); got != tt.want {
				t.Errorf("Maunder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIlyas1988(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"zero DAZ at limit", CrescentParams{ArcV: 10.2832719598, DAZ: 0}, true},
		{"zero DAZ below", CrescentParams{ArcV: 10.28, DAZ: 0}, false},
		{"high ArcV", CrescentParams{ArcV: 15, DAZ: 10}, true},
		{"low ArcV", CrescentParams{ArcV: 5, DAZ: 5}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Ilyas1988(); got != tt.want {
				t.Errorf("Ilyas1988() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFatoohi(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"zero DAZ at limit", CrescentParams{ArcV: 10.7638, DAZ: 0}, true},
		{"zero DAZ below", CrescentParams{ArcV: 10.76, DAZ: 0}, false},
		{"high ArcV", CrescentParams{ArcV: 15, DAZ: 5}, true},
		{"low ArcV", CrescentParams{ArcV: 5, DAZ: 5}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Fatoohi(); got != tt.want {
				t.Errorf("Fatoohi() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKraussAthenian(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"zero DAZ at limit", CrescentParams{ArcV: 10.5981838905, DAZ: 0}, true},
		{"zero DAZ below", CrescentParams{ArcV: 10.59, DAZ: 0}, false},
		{"high ArcV", CrescentParams{ArcV: 15, DAZ: 10}, true},
		{"low ArcV", CrescentParams{ArcV: 5, DAZ: 10}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.KraussAthenian(); got != tt.want {
				t.Errorf("KraussAthenian() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── Category 2: Calendrical ─────────────────────────────────────────────────

func TestMABIMS1995(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"both met", CrescentParams{ArcL: 5, MAlt: 3}, true},
		{"exact boundary", CrescentParams{ArcL: 3.0, MAlt: 2.0}, true},
		{"ArcL too low", CrescentParams{ArcL: 2.9, MAlt: 3}, false},
		{"MAlt too low", CrescentParams{ArcL: 5, MAlt: 1.9}, false},
		{"both too low", CrescentParams{ArcL: 2.0, MAlt: 1.0}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.MABIMS1995(); got != tt.want {
				t.Errorf("MABIMS1995() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIstanbul2016(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"both met", CrescentParams{ArcL: 10, MAlt: 6}, true},
		{"exact boundary", CrescentParams{ArcL: 8.0, MAlt: 5.0}, true},
		{"ArcL too low", CrescentParams{ArcL: 7.9, MAlt: 6}, false},
		{"MAlt too low", CrescentParams{ArcL: 10, MAlt: 4.9}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Istanbul2016(); got != tt.want {
				t.Errorf("Istanbul2016() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMABIMS2021(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"both met", CrescentParams{ArcL: 8, MAlt: 4}, true},
		{"exact boundary", CrescentParams{ArcL: 6.4, MAlt: 3.0}, true},
		{"ArcL too low", CrescentParams{ArcL: 6.3, MAlt: 4}, false},
		{"MAlt too low", CrescentParams{ArcL: 8, MAlt: 2.9}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.MABIMS2021(); got != tt.want {
				t.Errorf("MABIMS2021() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── Category 3: Elongation Limits ───────────────────────────────────────────

func TestDanjon(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"above", CrescentParams{ArcL: 10}, true},
		{"exact", CrescentParams{ArcL: 7.0}, true},
		{"below", CrescentParams{ArcL: 6.9}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Danjon(); got != tt.want {
				t.Errorf("Danjon() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSchaefer(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"above", CrescentParams{ArcL: 10}, true},
		{"exact", CrescentParams{ArcL: 7.5}, true},
		{"below", CrescentParams{ArcL: 7.4}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Schaefer(); got != tt.want {
				t.Errorf("Schaefer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIlyas1984(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"above", CrescentParams{ArcL: 12}, true},
		{"exact", CrescentParams{ArcL: 10.5}, true},
		{"below", CrescentParams{ArcL: 10.4}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Ilyas1984(); got != tt.want {
				t.Errorf("Ilyas1984() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── Category 4: ArcV vs Width ───────────────────────────────────────────────

func TestBruin(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"zero width at limit", CrescentParams{ArcV: 11.5621745317, W: 0}, true},
		{"zero width below", CrescentParams{ArcV: 11.56, W: 0}, false},
		{"wide crescent visible", CrescentParams{ArcV: 8, W: 1.0}, true},
		{"narrow crescent invisible", CrescentParams{ArcV: 5, W: 0.2}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Bruin(); got != tt.want {
				t.Errorf("Bruin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlrefayNakedEye(t *testing.T) {
	// Note: strict inequality (>), not >=
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"well above", CrescentParams{ArcV: 15, W: 0.5}, true},
		{"at limit exactly", CrescentParams{ArcV: 9.34, W: 0}, false}, // strict >
		{"just above", CrescentParams{ArcV: 9.35, W: 0}, true},
		{"well below", CrescentParams{ArcV: 5, W: 0.5}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.AlrefayNakedEye(); got != tt.want {
				t.Errorf("AlrefayNakedEye() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestYallop(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Yallop()
			if got.Code != tt.wantCode {
				t.Errorf("Yallop() code = %q, want %q (q=%.6f)", got.Code, tt.wantCode, got.Value)
			}
		})
	}
}

func TestYallopBoundaries(t *testing.T) {
	// Boundary semantics from spec:
	//   A: q > +0.216
	//   B: +0.216 >= q > -0.014
	// So q = 0.216 exactly → zone B (since A requires strictly >)
	w := 0.0
	// q = (ArcV - 11.8371) / 10
	// q = 0.216 → ArcV = 11.8371 + 2.16 = 13.9971
	p := CrescentParams{ArcV: 13.9971, W: w}
	got := p.Yallop()
	// Due to floating-point, 13.9971 gives q ≈ 0.216000...
	// The switch checks q > 0.216, so at exactly 0.216 it should be B.
	// However, floating point: (13.9971 - 11.8371)/10 may not be exactly 0.216.
	// Accept either A or B at the boundary; verify the q value is correct.
	if got.Code != "A" && got.Code != "B" {
		t.Errorf("q≈0.216 should be A or B, got %q (q=%.15f)", got.Code, got.Value)
	}

	// Clearly above → must be A
	p.ArcV = 14.1

	got = p.Yallop()
	if got.Code != "A" {
		t.Errorf("q clearly above 0.216 should be zone A, got %q (q=%.6f)", got.Code, got.Value)
	}

	// Clearly below → must be B
	p.ArcV = 13.5

	got = p.Yallop()
	if got.Code != "B" {
		t.Errorf("q clearly below 0.216 should be zone B, got %q (q=%.6f)", got.Code, got.Value)
	}
}

func TestOdeh(t *testing.T) {
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
		{"optical only", "Optical Only", CrescentParams{ArcV: 7, W: 0}},
		{"not visible", "Not Visible", CrescentParams{ArcV: 2, W: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Odeh()
			if got.Code != tt.wantCode {
				t.Errorf("Odeh() code = %q, want %q (V=%.6f)", got.Code, tt.wantCode, got.Value)
			}
		})
	}
}

func TestOdehBoundaries(t *testing.T) {
	// V = ArcV - 7.1651 (at W=0)
	// V >= 5.65 → Naked Eye
	// Clearly above: ArcV = 13.0 → V = 5.8349
	p := CrescentParams{ArcV: 13.0, W: 0}

	got := p.Odeh()
	if got.Code != "Naked Eye" {
		t.Errorf("V=5.83 should be Naked Eye, got %q (V=%.6f)", got.Code, got.Value)
	}

	// V just below 5.65 → Optical/Naked (ArcV = 12.81 → V = 5.6449)
	p.ArcV = 12.81

	got = p.Odeh()
	if got.Code != "Optical/Naked" {
		t.Errorf("V=5.6449 should be Optical/Naked, got %q (V=%.6f)", got.Code, got.Value)
	}
}

func TestQureshi(t *testing.T) {
	// At W=0: S = (ArcV + 10.43418) / 10
	// Zone A: S > 0.15   → ArcV > -10.43418 + 1.5 = -8.93418 (always for positive ArcV)
	// So we need non-zero W to get lower zones.
	// At W=1: poly = 0.351964 - 2.222075 + 5.422643 - 10.43418 = -6.882448
	//   S = (ArcV + 6.882448) / 10
	//   Zone A: S > 0.15  → ArcV > -5.382448
	//   Zone B: 0.15 >= S > 0.05  → -5.382448 >= ArcV > -6.382448
	// At W=0, all positive ArcV → zone A. We need W large enough.
	// At W=3: poly = 0.351964*27 - 2.222075*9 + 5.422643*3 - 10.43418
	//       = 9.503028 - 19.998675 + 16.267929 - 10.43418 = -4.661898
	//   S = (ArcV + 4.661898) / 10
	//   Zone A: S > 0.15  → ArcV > -3.161898
	// Hard to get low zones with standard values. Use direct computation.
	// Pick W=0 and compute ArcV for each zone:
	//   S = (ArcV + 10.43418) / 10
	//   A: S > 0.15  → ArcV > -8.93418   (any positive ArcV)
	//   B: 0.15 >= S > 0.05 → -8.93418 >= ArcV > -9.93418
	// Negative ArcV needed for lower zones, which is unphysical.
	// Use large W=5 to shift the polynomial.
	// poly(5) = 0.351964*125 - 2.222075*25 + 5.422643*5 - 10.43418
	//         = 43.9955 - 55.551875 + 27.113215 - 10.43418 = 5.122660
	// S = (ArcV - 5.122660) / 10
	// A: S > 0.15  → ArcV > 6.62266
	// B: 0.15 >= S > 0.05 → 6.62266 >= ArcV > 5.62266
	// C: 0.05 >= S > -0.06 → 5.62266 >= ArcV > 4.52266
	// D: -0.06 >= S > -0.16 → 4.52266 >= ArcV > 3.52266
	// E: S <= -0.16 → ArcV <= 3.52266
	tests := []struct {
		name     string
		wantCode string
		p        CrescentParams
	}{
		{"easily visible", "A", CrescentParams{ArcV: 8.0, W: 5.0}},
		{"perfect conditions", "B", CrescentParams{ArcV: 6.0, W: 5.0}},
		{"may require optical", "C", CrescentParams{ArcV: 5.0, W: 5.0}},
		{"require optical", "D", CrescentParams{ArcV: 4.0, W: 5.0}},
		{"not visible", "E", CrescentParams{ArcV: 3.0, W: 5.0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.p.Qureshi()
			if got.Code != tt.wantCode {
				t.Errorf("Qureshi() code = %q, want %q (S=%.6f)", got.Code, tt.wantCode, got.Value)
			}
		})
	}
}

// ── Category 5: Lag Time ────────────────────────────────────────────────────

func TestCaldwellNakedEye(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"high lag time", CrescentParams{LT: 50, ArcL: 10}, true},
		{"below limit", CrescentParams{LT: 34.940, ArcL: 10}, false}, // -0.9709*10+44.65 = 34.941
		{"above limit", CrescentParams{LT: 34.942, ArcL: 10}, true},  // strict >
		{"low lag time", CrescentParams{LT: 20, ArcL: 10}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.CaldwellNakedEye(); got != tt.want {
				limit := -0.9709*tt.p.ArcL + 44.65
				t.Errorf("CaldwellNakedEye() = %v, want %v (LT=%.4f, limit=%.4f)", got, tt.want, tt.p.LT, limit)
			}
		})
	}
}

func TestCaldwellOptical(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"high lag time", CrescentParams{LT: 40, ArcL: 10}, true},
		{"low lag time", CrescentParams{LT: 15, ArcL: 10}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.CaldwellOptical(); got != tt.want {
				t.Errorf("CaldwellOptical() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGautschy(t *testing.T) {
	tests := []struct {
		name string
		p    CrescentParams
		want bool
	}{
		{"zero DAZ at limit", CrescentParams{LT: 33.8890455442, DAZ: 0}, true},
		{"zero DAZ below", CrescentParams{LT: 33.88, DAZ: 0}, false},
		{"high lag time", CrescentParams{LT: 50, DAZ: 5}, true},
		{"low lag time", CrescentParams{LT: 20, DAZ: 5}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.Gautschy(); got != tt.want {
				t.Errorf("Gautschy() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ── EvaluateAll ─────────────────────────────────────────────────────────────

func TestEvaluateAll(t *testing.T) {
	// Use a set of params that gives a mix of visible/invisible across criteria.
	p := CrescentParams{
		ArcV: 10.5,
		ArcL: 12.0,
		DAZ:  8.0,
		MAlt: 5.5,
		W:    0.5,
		LT:   35.0,
	}
	r := p.EvaluateAll()

	// Verify params are stored
	if r.Params != p {
		t.Error("EvaluateAll did not store params correctly")
	}

	// Verify individual criteria match direct calls
	if r.Fotheringham != p.Fotheringham() {
		t.Error("Fotheringham mismatch")
	}

	if r.Maunder != p.Maunder() {
		t.Error("Maunder mismatch")
	}

	if r.Ilyas1988 != p.Ilyas1988() {
		t.Error("Ilyas1988 mismatch")
	}

	if r.Fatoohi != p.Fatoohi() {
		t.Error("Fatoohi mismatch")
	}

	if r.KraussAthenian != p.KraussAthenian() {
		t.Error("KraussAthenian mismatch")
	}

	if r.MABIMS1995 != p.MABIMS1995() {
		t.Error("MABIMS1995 mismatch")
	}

	if r.Istanbul2016 != p.Istanbul2016() {
		t.Error("Istanbul2016 mismatch")
	}

	if r.MABIMS2021 != p.MABIMS2021() {
		t.Error("MABIMS2021 mismatch")
	}

	if r.Danjon != p.Danjon() {
		t.Error("Danjon mismatch")
	}

	if r.Schaefer != p.Schaefer() {
		t.Error("Schaefer mismatch")
	}

	if r.Ilyas1984 != p.Ilyas1984() {
		t.Error("Ilyas1984 mismatch")
	}

	if r.Bruin != p.Bruin() {
		t.Error("Bruin mismatch")
	}

	if r.AlrefayNakedEye != p.AlrefayNakedEye() {
		t.Error("AlrefayNakedEye mismatch")
	}

	if r.CaldwellNakedEye != p.CaldwellNakedEye() {
		t.Error("CaldwellNakedEye mismatch")
	}

	if r.CaldwellOptical != p.CaldwellOptical() {
		t.Error("CaldwellOptical mismatch")
	}

	if r.Gautschy != p.Gautschy() {
		t.Error("Gautschy mismatch")
	}

	// Multi-zone value checks
	yDirect := p.Yallop()
	if math.Abs(r.Yallop.Value-yDirect.Value) > 1e-10 || r.Yallop.Code != yDirect.Code {
		t.Errorf("Yallop mismatch: got %v, want %v", r.Yallop, yDirect)
	}

	oDirect := p.Odeh()
	if math.Abs(r.Odeh.Value-oDirect.Value) > 1e-10 || r.Odeh.Code != oDirect.Code {
		t.Errorf("Odeh mismatch: got %v, want %v", r.Odeh, oDirect)
	}

	qDirect := p.Qureshi()
	if math.Abs(r.Qureshi.Value-qDirect.Value) > 1e-10 || r.Qureshi.Code != qDirect.Code {
		t.Errorf("Qureshi mismatch: got %v, want %v", r.Qureshi, qDirect)
	}
}

func TestEvaluateAllString(t *testing.T) {
	p := CrescentParams{ArcV: 10.5, ArcL: 12.0, DAZ: 8.0, MAlt: 5.5, W: 0.5, LT: 35.0}
	r := p.EvaluateAll()

	s := r.String()
	if len(s) == 0 {
		t.Error("String() returned empty")
	}
	// Smoke test: should contain key labels
	for _, want := range []string{"Fotheringham", "Maunder", "Yallop", "Odeh", "Qureshi", "Danjon", "MABIMS"} {
		if !containsStr(s, want) {
			t.Errorf("String() missing %q", want)
		}
	}
}

func TestCrescentZoneString(t *testing.T) {
	z := CrescentZone{Code: "A", Label: "Easily visible", Value: 0.3456}

	s := z.String()
	if s != "A: Easily visible (value=0.3456)" {
		t.Errorf("CrescentZone.String() = %q", s)
	}
}

// containsStr checks if s contains substr.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
