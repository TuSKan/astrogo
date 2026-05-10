package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// ── Lunar Crescent Visibility Criteria ───────────────────────────────────────
//
// This file implements 20 modern lunar crescent visibility criteria published
// between 1910 and 2021. Each criterion is a pure function of pre-computed
// topocentric parameters, returning either a boolean (visible/invisible) or
// a multi-zone classification.
//
// Reference:
//   Al-Jumaili M.H.A., Kamal M.S., Hussain S.A., Hanif N.H.H.M.,
//   "A Review on Modern Lunar Crescent Visibility Criterion",
//   Malaysian Journal of Science, Vol. 41(3), pp. 46–59, 2022.
//   https://mjs.um.edu.my/index.php/MJS/article/view/44110/18309

// CrescentParams holds the pre-computed topocentric parameters needed
// to evaluate lunar crescent visibility criteria.
//
// These values are typically derived from the Sun and Moon's topocentric
// positions at a specific observer location and time (usually shortly
// after sunset on the evening of potential first sighting).
type CrescentParams struct {
	// ArcV is the Arc of Vision: the angular difference in altitude
	// between the Sun (below horizon) and the Moon (above horizon),
	// measured in degrees. Also called ARCV in some literature.
	ArcV float64

	// ArcL is the Arc of Light or Elongation: the angular separation
	// between the Sun and Moon centers, measured in degrees.
	ArcL float64

	// DAZ is the Difference in Azimuth between the Sun and Moon,
	// measured in degrees.
	DAZ float64

	// MAlt is the Moon's altitude above the horizon, measured in degrees.
	MAlt float64

	// W is the topocentric crescent width, measured in arc minutes.
	// This depends on the Moon's semi-diameter and phase angle.
	W float64

	// LT is the Lag Time: the interval between sunset and moonset,
	// measured in minutes.
	LT float64
}

// ── Multi-Zone Classification ────────────────────────────────────────────────

// CrescentZone represents a visibility classification zone returned by
// multi-zone criteria (Yallop, Odeh, Qureshi).
type CrescentZone struct {
	// Code is the short classification identifier (e.g. "A", "Naked Eye").
	Code string

	// Label is the full human-readable description
	// (e.g. "Easily visible", "May need optical aid").
	Label string

	// Value is the computed discriminant parameter (q, V, or S).
	Value float64
}

// String returns a formatted representation: "Code: Label (value=X.XXXX)".
func (z CrescentZone) String() string {
	return fmt.Sprintf("%s: %s (value=%.4f)", z.Code, z.Label, z.Value)
}

// ── Category 1: Altitude & Azimuth Criteria (ArcV vs DAZ) ───────────────────

// Fotheringham evaluates the Fotheringham (1910) criterion.
// The crescent is visible if ArcV ≥ 12.0 − 0.008·DAZ.
//
// This is the earliest modern empirical criterion, based on ancient
// Babylonian observations compiled by Fotheringham.
func (p *CrescentParams) Fotheringham() bool {
	return p.ArcV >= 12.0-0.008*p.DAZ
}

// Maunder evaluates the Maunder (1911) criterion.
// The crescent is visible if ArcV ≥ 11.0 − 0.005·DAZ − 0.01·DAZ².
//
// Maunder refined Fotheringham's work using a quadratic fit to the
// visibility boundary in the (DAZ, ArcV) plane.
func (p *CrescentParams) Maunder() bool {
	return p.ArcV >= 11.0-0.005*p.DAZ-0.01*p.DAZ*p.DAZ
}

// Ilyas1988 evaluates the Ilyas (1988) criterion.
// The crescent is visible if ArcV ≥ f(DAZ), where f is a cubic polynomial.
//
// Ilyas extended the visibility boundary with additional data from tropical
// latitudes, producing a more permissive curve than Maunder.
func (p *CrescentParams) Ilyas1988() bool {
	d := p.DAZ
	limit := -0.0027356815*d - 0.0136648716*d*d + 0.0002119205*d*d*d + 10.2832719598

	return p.ArcV >= limit
}

// Fatoohi evaluates the Fatoohi et al. (1998) upper-limit criterion.
// The crescent is visible if ArcV ≥ f(DAZ), where f is a cubic polynomial.
//
// This upper-limit curve was derived from Babylonian and Islamic historical
// records. Observations above this curve are certainly visible.
func (p *CrescentParams) Fatoohi() bool {
	d := p.DAZ
	limit := 10.7638 + 0.0356*d - 0.0164*d*d + 0.0004*d*d*d

	return p.ArcV >= limit
}

// KraussAthenian evaluates the Krauss (2012) Athenian criterion.
// The crescent is visible if ArcV ≥ f(DAZ), where f is a cubic polynomial.
//
// Krauss derived this curve from ancient Athenian calendar data to model
// historical lunar month beginnings.
func (p *CrescentParams) KraussAthenian() bool {
	d := p.DAZ
	limit := 0.0291254840*d - 0.0098347831*d*d + 0.0000475196*d*d*d + 10.5981838905

	return p.ArcV >= limit
}

// ── Category 2: Arc of Light & Moon Altitude (Calendrical) ───────────────────

// MABIMS1995 evaluates the original MABIMS (1995) criterion used by
// Brunei, Indonesia, Malaysia, and Singapore for Islamic calendar determination.
// The crescent is visible if ArcL ≥ 3° AND MAlt ≥ 2°.
//
// This is the most permissive calendrical criterion and has been
// superseded by MABIMS2021 for official use.
func (p *CrescentParams) MABIMS1995() bool {
	return p.ArcL >= 3.0 && p.MAlt >= 2.0
}

// Istanbul2016 evaluates the Istanbul (2016) criterion adopted by the
// Organisation of Islamic Cooperation (OIC).
// The crescent is visible if ArcL ≥ 8° AND MAlt ≥ 5°.
//
// This is the most conservative calendrical criterion, requiring
// both substantial elongation and significant moon altitude.
func (p *CrescentParams) Istanbul2016() bool {
	return p.ArcL >= 8.0 && p.MAlt >= 5.0
}

// MABIMS2021 evaluates the revised MABIMS (2021) criterion.
// The crescent is visible if ArcL ≥ 6.4° AND MAlt ≥ 3°.
//
// This updated criterion replaced MABIMS1995 after extensive review
// of observational data from Southeast Asian countries.
func (p *CrescentParams) MABIMS2021() bool {
	return p.ArcL >= 6.4 && p.MAlt >= 3.0
}

// ── Category 3: Singular Elongation Limits ───────────────────────────────────

// Danjon evaluates the Danjon (1936) limit.
// The crescent is visible if ArcL ≥ 7°.
//
// The Danjon limit is the minimum angular separation below which no
// crescent visibility is possible due to the intense glare of the Sun.
// This serves as the absolute physical lower bound for all criteria.
func (p *CrescentParams) Danjon() bool {
	return p.ArcL >= 7.0
}

// Schaefer evaluates the Schaefer (1991) elongation limit.
// The crescent is visible if ArcL ≥ 7.5°.
//
// Schaefer refined the Danjon limit using modern photometric models
// of atmospheric scattering and human visual threshold.
func (p *CrescentParams) Schaefer() bool {
	return p.ArcL >= 7.5
}

// Ilyas1984 evaluates the Ilyas (1984) naked-eye elongation limit.
// The crescent is visible if ArcL ≥ 10.5°.
//
// Ilyas proposed this as the practical minimum elongation for reliable
// naked-eye sighting under average observing conditions.
func (p *CrescentParams) Ilyas1984() bool {
	return p.ArcL >= 10.5
}

// ── Category 4: Arc of Vision & Lunar Width (ArcV vs W) ─────────────────────

// Bruin evaluates the Bruin (1977) criterion.
// The crescent is visible if ArcV ≥ f(W), where f is a cubic polynomial
// in the crescent width W (arc minutes).
//
// Bruin was the first to incorporate the crescent width into the
// visibility criterion, recognizing that wider crescents are easier
// to detect against twilight sky brightness.
func (p *CrescentParams) Bruin() bool {
	w := p.W
	limit := 11.5621745317 - 7.944238328*w + 3.2608487770*w*w - 0.4559413249*w*w*w

	return p.ArcV >= limit
}

// AlrefayNakedEye evaluates the Al-Refay et al. (2018) naked-eye criterion.
// The crescent is visible if ArcV > f(W), where f is a cubic polynomial
// in the crescent width W (arc minutes).
//
// Note: this criterion uses strict inequality (>), unlike most others
// that use ≥.
func (p *CrescentParams) AlrefayNakedEye() bool {
	w := p.W
	limit := 9.34 - 4.51*w + 3.3*w*w - 1.01*w*w*w

	return p.ArcV > limit
}

// Yallop evaluates the Yallop (1998) multi-zone criterion.
// It calculates the q parameter from ArcV and W, then classifies the
// observation into one of six visibility zones (A through F).
//
// The q parameter is:
//
//	q = (ArcV − 11.8371 + 6.3226·W − 0.7319·W² + 0.1018·W³) / 10
//
// Zones:
//
//	A: Easily visible               (q > +0.216)
//	B: Visible under perfect cond.  (+0.216 ≥ q > −0.014)
//	C: May need optical aid         (−0.014 ≥ q > −0.160)
//	D: Will need optical aid        (−0.160 ≥ q > −0.232)
//	E: Not visible with telescope   (−0.232 ≥ q > −0.293)
//	F: Not visible, below Danjon    (−0.293 ≥ q)
//
// This is one of the most widely used criteria in modern lunar calendar
// determination and has been adopted by many national observatories.
func (p *CrescentParams) Yallop() CrescentZone {
	w := p.W
	q := (p.ArcV - 11.8371 + 6.3226*w - 0.7319*w*w + 0.1018*w*w*w) / 10.0

	switch {
	case q > 0.216:
		return CrescentZone{Code: "A", Label: "Easily visible", Value: q}
	case q > -0.014:
		return CrescentZone{Code: "B", Label: "Visible under perfect conditions", Value: q}
	case q > -0.160:
		return CrescentZone{Code: "C", Label: "May need optical aid", Value: q}
	case q > -0.232:
		return CrescentZone{Code: "D", Label: "Will need optical aid", Value: q}
	case q > -0.293:
		return CrescentZone{Code: "E", Label: "Not visible with telescope", Value: q}
	default:
		return CrescentZone{Code: "F", Label: "Not visible, below Danjon limit", Value: q}
	}
}

// Odeh evaluates the Odeh (2004) multi-zone criterion.
// It calculates the V parameter from ArcV and W, then classifies the
// observation into one of four visibility zones.
//
// The V parameter is:
//
//	V = ArcV − (−0.1018·W³ + 0.7319·W² − 6.3226·W + 7.1651)
//
// Zones:
//
//	Naked Eye                  (V ≥ 5.65)
//	Optical Aid / Maybe Naked  (5.65 > V ≥ 2.0)
//	Optical Aid Only           (2.0 > V ≥ −0.96)
//	Not Visible                (V < −0.96)
//
// Odeh's criterion was developed using a large database of over 700
// observations and is considered one of the most reliable modern criteria.
func (p *CrescentParams) Odeh() CrescentZone {
	w := p.W
	v := p.ArcV - (-0.1018*w*w*w + 0.7319*w*w - 6.3226*w + 7.1651)

	switch {
	case v >= 5.65:
		return CrescentZone{Code: "Naked Eye", Label: "Visible to naked eye", Value: v}
	case v >= 2.0:
		return CrescentZone{Code: "Optical/Naked", Label: "Optical aid, may be seen by naked eye", Value: v}
	case v >= -0.96:
		return CrescentZone{Code: "Optical Only", Label: "Visible only with optical aid", Value: v}
	default:
		return CrescentZone{Code: "Not Visible", Label: "Not visible", Value: v}
	}
}

// Qureshi evaluates the Qureshi (2010) multi-zone criterion.
// It calculates the S parameter from ArcV and W, then classifies the
// observation into one of five visibility zones.
//
// The S parameter is:
//
//	S = (ArcV − 0.351964·W³ + 2.222075·W² − 5.422643·W + 10.43418) / 10
//
// Zones:
//
//	Easily visible                 (S > 0.15)
//	Visible under perfect cond.    (0.15 ≥ S > 0.05)
//	May require optical aid        (0.05 ≥ S > −0.06)
//	Require optical aid            (−0.06 ≥ S > −0.16)
//	Not visible with optical aid   (S ≤ −0.16)
func (p *CrescentParams) Qureshi() CrescentZone {
	w := p.W
	s := (p.ArcV - 0.351964*w*w*w + 2.222075*w*w - 5.422643*w + 10.43418) / 10.0

	switch {
	case s > 0.15:
		return CrescentZone{Code: "A", Label: "Easily visible", Value: s}
	case s > 0.05:
		return CrescentZone{Code: "B", Label: "Visible under perfect conditions", Value: s}
	case s > -0.06:
		return CrescentZone{Code: "C", Label: "May require optical aid", Value: s}
	case s > -0.16:
		return CrescentZone{Code: "D", Label: "Require optical aid", Value: s}
	default:
		return CrescentZone{Code: "E", Label: "Not visible with optical aid", Value: s}
	}
}

// ── Category 5: Lag Time Criteria ────────────────────────────────────────────

// CaldwellNakedEye evaluates the Caldwell & Laney (2011) naked-eye criterion.
// The crescent is visible to the naked eye if LT > −0.9709·ArcL + 44.65.
//
// Lag time is the interval (in minutes) between sunset and moonset.
// Larger lag times give more time for the sky to darken while the
// Moon is still above the horizon.
func (p *CrescentParams) CaldwellNakedEye() bool {
	return p.LT > -0.9709*p.ArcL+44.65
}

// CaldwellOptical evaluates the Caldwell & Laney (2011) optical-aided criterion.
// The crescent is visible with optical aid if LT > −1.9230·ArcL + 43.13.
//
// The optical-aided boundary permits smaller lag times than the
// naked-eye criterion at the same elongation.
func (p *CrescentParams) CaldwellOptical() bool {
	return p.LT > -1.9230*p.ArcL+43.13
}

// Gautschy evaluates the Gautschy (2014) criterion.
// The crescent is visible if LT ≥ f(DAZ), where f is a cubic polynomial
// in the difference of azimuth.
//
// Gautschy derived this curve from ancient Babylonian calendar records
// to model historical first-visibility practices.
func (p *CrescentParams) Gautschy() bool {
	d := p.DAZ
	limit := 0.3342328913*d - 0.0715608980*d*d + 0.0009924422*d*d*d + 33.8890455442

	return p.LT >= limit
}

// ── Aggregate Evaluation ─────────────────────────────────────────────────────

// CrescentResult holds the evaluation of all 20 visibility criteria
// against a single set of crescent parameters.
type CrescentResult struct {
	Qureshi          CrescentZone
	Odeh             CrescentZone
	Yallop           CrescentZone
	Params           CrescentParams
	KraussAthenian   bool
	Ilyas1984        bool
	MABIMS1995       bool
	Istanbul2016     bool
	MABIMS2021       bool
	Danjon           bool
	Schaefer         bool
	Fatoohi          bool
	Bruin            bool
	AlrefayNakedEye  bool
	Ilyas1988        bool
	Maunder          bool
	Fotheringham     bool
	CaldwellNakedEye bool
	CaldwellOptical  bool
	Gautschy         bool
}

// EvaluateAll runs all 20 lunar crescent visibility criteria against the
// given parameters and returns the complete set of results.
//
// This is a convenience function that avoids calling each criterion
// individually. All criteria are evaluated regardless of input validity;
// the caller is responsible for ensuring the parameters are physically
// meaningful (e.g., positive crescent width, non-negative lag time).
func (p *CrescentParams) EvaluateAll() CrescentResult {
	return CrescentResult{
		Params: *p,

		// Category 1
		Fotheringham:   p.Fotheringham(),
		Maunder:        p.Maunder(),
		Ilyas1988:      p.Ilyas1988(),
		Fatoohi:        p.Fatoohi(),
		KraussAthenian: p.KraussAthenian(),

		// Category 2
		MABIMS1995:   p.MABIMS1995(),
		Istanbul2016: p.Istanbul2016(),
		MABIMS2021:   p.MABIMS2021(),

		// Category 3
		Danjon:    p.Danjon(),
		Schaefer:  p.Schaefer(),
		Ilyas1984: p.Ilyas1984(),

		// Category 4
		Bruin:           p.Bruin(),
		AlrefayNakedEye: p.AlrefayNakedEye(),
		Yallop:          p.Yallop(),
		Odeh:            p.Odeh(),
		Qureshi:         p.Qureshi(),

		// Category 5
		CaldwellNakedEye: p.CaldwellNakedEye(),
		CaldwellOptical:  p.CaldwellOptical(),
		Gautschy:         p.Gautschy(),
	}
}

// String returns a formatted multi-line summary of all criteria evaluations.
func (r CrescentResult) String() string {
	yn := func(b bool) string {
		if b {
			return "Visible"
		}

		return "Not visible"
	}

	return fmt.Sprintf(`Lunar Crescent Visibility Evaluation
═══════════════════════════════════════════════════════════════
Input Parameters:
  ArcV = %.4f°    ArcL = %.4f°    DAZ = %.4f°
  MAlt = %.4f°    W = %.4f'       LT = %.2f min

Category 1: Altitude & Azimuth (ArcV vs DAZ)
  Fotheringham (1910):    %s
  Maunder (1911):         %s
  Ilyas (1988):           %s
  Fatoohi (1998):         %s
  Krauss Athenian (2012): %s

Category 2: Calendrical (ArcL + MAlt)
  MABIMS (1995):          %s
  Istanbul (2016):        %s
  MABIMS (2021):          %s

Category 3: Elongation Limits
  Danjon (1936):          %s
  Schaefer (1991):        %s
  Ilyas (1984):           %s

Category 4: ArcV vs Crescent Width
  Bruin (1977):           %s
  Al-Refay (2018):        %s
  Yallop (1998):          %s
  Odeh (2004):            %s
  Qureshi (2010):         %s

Category 5: Lag Time
  Caldwell Naked Eye:     %s
  Caldwell Optical:       %s
  Gautschy (2014):        %s`,
		r.Params.ArcV, r.Params.ArcL, r.Params.DAZ,
		r.Params.MAlt, r.Params.W, r.Params.LT,
		yn(r.Fotheringham), yn(r.Maunder), yn(r.Ilyas1988),
		yn(r.Fatoohi), yn(r.KraussAthenian),
		yn(r.MABIMS1995), yn(r.Istanbul2016), yn(r.MABIMS2021),
		yn(r.Danjon), yn(r.Schaefer), yn(r.Ilyas1984),
		yn(r.Bruin), yn(r.AlrefayNakedEye),
		r.Yallop.String(), r.Odeh.String(), r.Qureshi.String(),
		yn(r.CaldwellNakedEye), yn(r.CaldwellOptical), yn(r.Gautschy),
	)
}

// ── Ephemeris Integration ───────────────────────────────────────────────────

// NewCrescentParams computes topocentric lunar crescent parameters at the
// given time and observer location using the provided ephemeris.
//
// The returned parameters are suitable for direct evaluation with all
// 20 crescent visibility criteria. The time should normally be shortly
// after local sunset on the evening of potential first sighting.
//
// The crescent width W is approximated using the mean lunar semi-diameter
// (15.5 arc-minutes) and the geocentric phase angle. The lag time LT is
// estimated from the Moon's altitude and the diurnal rotation rate.
// For higher fidelity, compute moonset time directly and pass a manually
// constructed CrescentParams.
//
// Example:
//
//	p, err := plan.NewCrescentParams(sunsetTime, jerusalem, prov)
//	if err != nil { ... }
//	result := p.EvaluateAll()
//	fmt.Println(result.String())
func NewCrescentParams(t time.Time, loc *coord.Geodetic, prov eph.Provider) (CrescentParams, error) {
	// Get geocentric ICRS positions for Sun and Moon
	sunPos, err := eph.Position(prov, eph.Sun, t)
	if err != nil {
		return CrescentParams{}, fmt.Errorf("crescent: sun position: %w", err)
	}

	moonPos, err := eph.Position(prov, eph.Moon, t)
	if err != nil {
		return CrescentParams{}, fmt.Errorf("crescent: moon position: %w", err)
	}

	sunICRS, err := eph.ToICRS(sunPos)
	if err != nil {
		return CrescentParams{}, fmt.Errorf("crescent: sun ICRS: %w", err)
	}

	moonICRS, err := eph.ToICRS(moonPos)
	if err != nil {
		return CrescentParams{}, fmt.Errorf("crescent: moon ICRS: %w", err)
	}

	// Topocentric AltAz for both bodies
	ctx := coord.NewContext(t, loc, atmosphere.Atmosphere{})

	sunAltAz, err := ctx.ICRSToAltAz(sunICRS)
	if err != nil {
		return CrescentParams{}, fmt.Errorf("crescent: sun altaz: %w", err)
	}

	moonAltAz, err := ctx.ICRSToAltAz(moonICRS)
	if err != nil {
		return CrescentParams{}, fmt.Errorf("crescent: moon altaz: %w", err)
	}

	sunAlt := sunAltAz.Alt().Degrees()
	moonAlt := moonAltAz.Alt().Degrees()
	sunAz := sunAltAz.Az().Degrees()
	moonAz := moonAltAz.Az().Degrees()

	// ArcV: altitude difference (Moon − Sun)
	arcV := moonAlt - sunAlt

	// DAZ: absolute azimuth difference
	daz := math.Abs(moonAz - sunAz)
	if daz > 180 {
		daz = 360 - daz
	}

	// ArcL: geocentric angular separation (elongation)
	arcL := coord.Separation(moonICRS, sunICRS).Degrees()

	// W: topocentric crescent width (arc minutes)
	// W ≈ SD × (1 − cos(elongation)), where SD ≈ 15.5' (mean lunar semi-diameter)
	const moonSD = 15.5 // arc minutes

	w := moonSD * (1.0 - math.Cos(arcL*math.Pi/180.0))

	// LT: lag time estimate (minutes)
	// Approximate from Moon altitude and diurnal rotation rate (~15°/hr).
	// For production use, compute actual moonset time.
	lt := 0.0
	if moonAlt > 0 {
		lt = moonAlt / (15.0 / 60.0) // minutes
	}

	return CrescentParams{
		ArcV: arcV,
		ArcL: arcL,
		DAZ:  daz,
		MAlt: moonAlt,
		W:    w,
		LT:   lt,
	}, nil
}
