package plan

import (
	"math"
	"slices"
	"testing"
)

// The tests in this file hold the meteor shower model to what observers
// counted (#586), where meteor_activity_test.go holds it to its sources. The
// model is one year for every year, so what it can be held to is the
// typical return, and the tolerance is the returns' own scatter.
//
// Two showers are held to it, the two Halleyids, because for them Egal et
// al. (2020, A&A 640, A58) give what was not found for the other seven: each
// year's maximum from one re-analysis of IMO's visual data, and an average
// profile fitted to visual counts.

// observedMaximum is one return's maximum: the Sun's J2000 longitude there,
// in degrees, and the ZHR.
type observedMaximum struct {
	year        int
	lambda, zhr float64
}

// egalMaxima are the yearly maxima of Egal et al. (2020) Appendix D, the
// rows of their own re-analysis of IMO's Visual Meteor Database, 2001–2019.
// A maximum printed as a range (2003, 44.1–44.4) is its midpoint, and a year
// printed with two maxima contributes the first.
//
// Left out, on the paper's own word: the η-Aquariids' 2013 outburst, ZHR 130
// (their Sect. 6.1.1), and the Orionids of 2002–2013, when "all techniques
// agree" their activity was enhanced, to a ZHR of 80 (Sect. 6.3.1). Two
// η-Aquariid rows are left out as misprints: 2015 prints a ZHR of 63 ± 120,
// and 2016 one of 3 ± 17.
var egalMaxima = map[string][]observedMaximum{
	"eta_aquariids": {
		{2001, 45.5, 67}, {2002, 46, 54}, {2003, 44.25, 45}, {2005, 45.6, 83},
		{2006, 44.4, 64}, {2007, 45.4, 62}, {2008, 45.8, 58}, {2009, 45.7, 54},
		{2010, 45.3, 60}, {2011, 45.3, 63}, {2012, 45, 80}, {2014, 45.65, 62},
		{2017, 45.32, 68}, {2018, 44.88, 71}, {2019, 46.7, 54},
	},
	"orionids": {
		{2001, 207.9, 30}, {2014, 208.4, 21}, {2015, 207.7, 21}, {2016, 208.7, 19},
		{2017, 207.5, 28}, {2018, 207.3, 25}, {2019, 209.0, 25},
	},
}

// imoEtaAquariidPeaks is IMO's own list of recent η-Aquariid peak ZHRs, in
// its 2027 Meteor Shower Calendar, for 2017–2025; 2026's is marked
// preliminary and left out. The calendar notes the rate has fallen since
// 2008, which is why it is held to separately from Egal et al.'s years.
var imoEtaAquariidPeaks = []float64{75, 60, 50, 50, 45, 42, 40, 45, 45}

func medianOf(xs []float64) float64 {
	s := slices.Sorted(slices.Values(xs))
	n := len(s)

	if n%2 == 1 {
		return s[n/2]
	}

	return (s[n/2-1] + s[n/2]) / 2
}

// sampleSD is the standard deviation of xs about their mean, with n − 1.
func sampleSD(xs []float64) float64 {
	var mean float64
	for _, x := range xs {
		mean += x
	}

	mean /= float64(len(xs))

	var ss float64
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}

	return math.Sqrt(ss / float64(len(xs)-1))
}

func log10s(xs []float64) []float64 {
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = math.Log10(x)
	}

	return out
}

// TestMeteorMaximaAgreeWithObservedReturns holds each Halleyid's maximum,
// its time and its rate, to the median of the observed returns, within twice
// the returns' standard deviation about it: in λ☉ for the time, and in
// log ZHR for the rate, a factor either way.
//
// Measured: the maximum is 0.10° after the median for both, against a
// scatter of 0.61° (η-Aquariids) and 0.64° (Orionids). The ZHR at it, IMO's,
// is 0.81 of the median η-Aquariid return of 2001–2019 and 1.11 of IMO's
// 2017–2025, the shower having faded between them, and 0.80 of the median
// Orionid return outside the enhanced years. The bounds, twice the returns'
// scatter, are ×1.37, ×1.48 and ×1.40.
func TestMeteorMaximaAgreeWithObservedReturns(t *testing.T) {
	t.Parallel()

	for key, returns := range egalMaxima {
		m := meteorShowers[key]

		var lambdas, zhrs []float64
		for _, r := range returns {
			lambdas = append(lambdas, r.lambda)
			zhrs = append(zhrs, r.zhr)
		}

		med, sd := medianOf(lambdas), sampleSD(lambdas)
		if d := m.PeakSolarLongitude - med; math.Abs(d) > 2*sd {
			t.Errorf("%s: maximum at λ☉ %g°, %+.2f° from the median of %d returns, %.2f°; the bound, twice their scatter, is %.2f°",
				key, m.PeakSolarLongitude, d, len(returns), med, 2*sd)
		}

		holdRate(t, key+" (Egal et al. 2020)", m.ZHR, zhrs)
	}

	holdRate(t, "eta_aquariids (IMO 2017–2025)", meteorShowers["eta_aquariids"].ZHR, imoEtaAquariidPeaks)
}

func holdRate(t *testing.T, label string, zhr float64, observed []float64) {
	t.Helper()

	logs := log10s(observed)
	med, sd := medianOf(logs), sampleSD(logs)

	if d := math.Log10(zhr) - med; math.Abs(d) > 2*sd {
		t.Errorf("%s: ZHR %g is %.2f of the median of %d returns, %.0f; the bound, twice their scatter, is ×%.2f",
			label, zhr, math.Pow(10, d), len(observed), math.Pow(10, med), math.Pow(10, 2*sd))
	}
}

// averageProfile is a shower's average activity profile as Egal et al.
// (2020) Table 2 fits it: a plateau from λ☉ start to end, across which the
// ZHR goes linearly from zhrStart to zhrEnd, with Jenniskens' exponential on
// either side, of slope rise before the plateau and fall after it (their
// B_p2 and B_m2).
type averageProfile struct {
	start, end       float64
	zhrStart, zhrEnd float64
	rise, fall       float64
}

// crossings returns where p falls to level times its maximum, before and
// after the plateau, for a level the plateau does not reach down to.
func (p averageProfile) crossings(level float64) (before, after float64) {
	z := level * math.Max(p.zhrStart, p.zhrEnd)

	return p.start - math.Log10(p.zhrStart/z)/p.rise, p.end + math.Log10(p.zhrEnd/z)/p.fall
}

// visualAverages are Egal et al.'s fits to the visual profiles, their
// "VMDB" rows. For the η-Aquariids that profile is IMO's average of
// 1988–2007 (Rendtel & Arlt 2008, their Fig. 4), and for the Orionids the
// Visual Meteor Database's of 2002–2019 (their Fig. 5). Only the visual
// profiles are used: a ZHR is a visual count, and the radar and video
// profiles of the same table sample fainter meteors.
var visualAverages = map[string]averageProfile{
	"eta_aquariids": {44.4, 46.4, 70, 64, 0.121, 0.106},
	"orionids":      {207.7, 210.5, 40, 33, 0.120, 0.168},
}

// profileFactor is how far the model's activity may be from the observed
// average profile's where that profile crosses half, a fifth and a tenth of
// its maximum.
//
// It is a factor of two, wider than the bound on the rate at the maximum
// (×1.37 to ×1.48), because the model is one exponential either side of a
// single maximum and both average profiles have a plateau, which no such
// curve follows. Measured, the model is within ×1.43 of the Orionids'
// everywhere (0.70 at half maximum after the plateau), and of the
// η-Aquariids' within ×1.07 at half maximum, ×1.41 at a fifth and ×1.78 at
// a tenth: Jenniskens' symmetric fit rises more gently than every one of
// Egal et al.'s η-Aquariid profiles, visual, video and radar.
//
// A flat profile, as before #572, is off by the inverse of the level: ×2,
// ×5 and ×10.
const profileFactor = 2

// TestMeteorProfilesFollowObservedAverages holds ZHRAt's profile to the
// observed average profile, at the λ☉ where that profile crosses half, a
// fifth and a tenth of its maximum, on both sides (#586). The rate at the
// maximum is held to the observed returns by
// TestMeteorMaximaAgreeWithObservedReturns; this holds only the shape.
func TestMeteorProfilesFollowObservedAverages(t *testing.T) {
	t.Parallel()

	for key, p := range visualAverages {
		m := meteorShowers[key]

		for _, level := range []float64{0.5, 0.2, 0.1} {
			before, after := p.crossings(level)

			for _, lambda := range []float64{before, after} {
				ratio := m.zhrAtSolarLongitude(lambda) / m.ZHR / level
				if ratio > profileFactor || ratio < 1/profileFactor {
					t.Errorf("%s at λ☉ %.2f°, where the observed average is %g of its maximum: the model's activity is %.2f of that",
						key, lambda, level, ratio)
				}
			}
		}
	}
}
