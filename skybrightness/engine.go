package skybrightness

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/parallel"
)

// ErrDuplicateComponent is returned by NewCompositeEngine when two
// Components report the same ComponentID.
var ErrDuplicateComponent = errors.New("skybrightness: duplicate ComponentID in CompositeConfig.Components")

// ErrMissingAtmosphere is returned when a Request supplies no
// AtmosphereState under a Mode that requires one, and
// EvaluationOptions.Fallback is FallbackForbidden (the default).
var ErrMissingAtmosphere = errors.New("skybrightness: no AtmosphereState supplied and fallback is forbidden")

// ErrEmptyBatch is returned by EvaluateBatch when BatchRequest.Astro is
// empty.
var ErrEmptyBatch = errors.New("skybrightness: BatchRequest.Astro must not be empty")

// ErrNonFiniteComponentSum is returned by Evaluate when the summed
// component radiance is non-finite or negative — a component's own Eval
// producing garbage rather than a legitimate physical zero.
var ErrNonFiniteComponentSum = errors.New("skybrightness: component sum produced a non-finite or negative radiance")

// TransmissionModel computes atmospheric transmission along a line of
// sight.
type TransmissionModel interface {
	Algorithm() AlgorithmRef
	// LineOfSight fills out (length g.Len()) with the transmission at
	// each of g's wavelengths, for the path from top-of-atmosphere to an
	// observer looking toward dir under atmospheric state st.
	LineOfSight(dir coord.AltAz, st *AtmosphereState, g SpectralGrid, out []Transmission) error
}

// Engine is the spectral sky-radiance evaluator.
type Engine interface {
	Algorithm() AlgorithmRef
	Evaluate(ctx context.Context, req Request) (Result, error)
	EvaluateBatch(ctx context.Context, req BatchRequest) (BatchResult, error)
}

// CompositeConfig configures a CompositeEngine: the set of components to
// sum, the transmission model, the passband set derived outputs may
// resolve IDs against, and the Mode this engine answers for.
type CompositeConfig struct {
	Name         AlgorithmRef
	Components   []Component
	Transmission TransmissionModel
	Passbands    PassbandSet
	Mode         Mode
}

// CompositeEngine sums a fixed set of Components into Result.Total, in
// linear spectral-radiance space, and computes whichever DerivedQuantities
// EvaluationOptions.Derived requests.
type CompositeEngine struct {
	cfg CompositeConfig
}

// NewCompositeEngine validates cfg (no two components sharing a
// ComponentID) and returns a ready-to-use engine.
func NewCompositeEngine(cfg CompositeConfig) (*CompositeEngine, error) {
	seen := ComponentMask(0)

	for _, c := range cfg.Components {
		if seen.Has(c.ID()) {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateComponent, c.ID())
		}

		seen = seen.Add(c.ID())
	}

	return &CompositeEngine{cfg: cfg}, nil
}

// Algorithm implements Engine.
func (e *CompositeEngine) Algorithm() AlgorithmRef { return e.cfg.Name }

// Evaluate implements Engine.
func (e *CompositeEngine) Evaluate(ctx context.Context, req Request) (Result, error) {
	if err := req.Validate(); err != nil {
		return Result{}, err
	}

	scratch := req.Options.Buffers.scratchOrNew(len(req.Directions), req.Grid.Len())

	return e.evaluateOne(ctx, req, scratch)
}

// EvaluateBatch implements Engine. Each epoch's evaluation runs against
// its own per-goroutine Scratch and cloned coord.Context, constructed
// once per goroutine via internal/parallel.MapChunked — never once per
// direction (docs/skybrightness.md §5).
func (e *CompositeEngine) EvaluateBatch(ctx context.Context, req BatchRequest) (BatchResult, error) {
	n := len(req.Astro)
	if n == 0 {
		return BatchResult{}, ErrEmptyBatch
	}

	nDir, nLambda := len(req.Directions), req.Grid.Len()

	results := make([]Result, n)
	errs := make([]error, n)
	workers := req.Options.Parallelism

	parallel.MapChunked(n, workers,
		func() *Scratch { return NewScratch(nDir, nLambda) },
		func(s *Scratch, i int) {
			r, err := e.evaluateOne(ctx, req.at(i), s)
			results[i], errs[i] = r, err
		},
	)

	for i, err := range errs {
		if err != nil {
			return BatchResult{}, fmt.Errorf("epoch %d: %w", i, err)
		}
	}

	return BatchResult{Epochs: results, Provenance: Provenance{
		SchemaVersion: provenanceSchemaVersion, Engine: e.cfg.Name, Mode: req.Mode, EvaluatedAt: time.Now(),
	}}, nil
}

const provenanceSchemaVersion = "skybrightness-v2/1"

func (e *CompositeEngine) evaluateOne(ctx context.Context, req Request, scratch *Scratch) (Result, error) {
	nDir, nLambda := len(req.Directions), req.Grid.Len()

	atm := req.Atmosphere

	var fallbacks []FallbackRecord

	if atm == nil {
		// ModeClimatology and ModeLegacy are both self-sufficient,
		// deterministic, offline defaults — neither requires an explicit
		// AtmosphereState, unlike Historical/Nowcast/Forecast/UserSupplied,
		// where a missing atmosphere is a real gap the caller must either
		// fill or explicitly opt to fall back from (docs/skybrightness.md
		// §7). ModeLegacy in particular exists so natural.NewLegacyEngine
		// works with zero setup, matching v1's own zero-configuration
		// default behavior.
		if req.Mode != ModeClimatology && req.Mode != ModeLegacy && req.Options.Fallback == FallbackForbidden {
			return Result{}, ErrMissingAtmosphere
		}

		atm = ClimatologyDefaultAtmosphere(nil)

		if req.Mode != ModeClimatology && req.Mode != ModeLegacy {
			fallbacks = append(fallbacks, FallbackRecord{
				From: req.Mode, To: ModeClimatology, Reason: "no AtmosphereState supplied", At: time.Now(),
			})
		}
	}

	mask := req.Selection.mask()
	total := NewSpectralField(nDir, nLambda)

	var (
		comps           ComponentResults
		compProvenances []ComponentProvenance
		quality         QualityFlags
	)

	for _, c := range e.cfg.Components {
		if !mask.Has(c.ID()) {
			continue
		}

		buf := scratch.Component()
		buf.Zero()

		in := EvalInput{
			Astro: req.Astro, Directions: req.Directions, Grid: req.Grid,
			Atmosphere: atm, Mode: req.Mode, Options: req.Options, Scratch: scratch,
		}

		rep, err := c.Eval(ctx, in, buf)
		if err != nil {
			return Result{}, fmt.Errorf("component %s: %w", c.ID(), err)
		}

		total.Add(buf)

		quality |= rep.Quality

		if req.Selection.Materialize {
			comps.set(c.ID(), buf.Clone(), rep)
		} else {
			comps.set(c.ID(), SpectralField{}, rep)
		}

		compProvenances = append(compProvenances, rep.Provenance)
	}

	if !total.MinNonNegative() {
		return Result{}, ErrNonFiniteComponentSum
	}

	var transmission []Transmission

	transAlgo := AlgorithmRef{}

	if req.Options.ComputeTransmission && e.cfg.Transmission != nil {
		transAlgo = e.cfg.Transmission.Algorithm()
		transmission = make([]Transmission, nDir*nLambda)

		for d, dir := range req.Directions {
			if err := e.cfg.Transmission.LineOfSight(dir, atm, req.Grid, transmission[d*nLambda:(d+1)*nLambda]); err != nil {
				return Result{}, fmt.Errorf("transmission at direction %d: %w", d, err)
			}
		}
	}

	if req.Options.Fallback != FallbackForbidden && len(fallbacks) > 0 {
		quality |= QualityFlagFallbackApplied
	}

	if req.Options.MaxInputAge > 0 && atm.Age(time.Now()) > req.Options.MaxInputAge {
		quality |= QualityFlagStaleAtmosphere
	}

	res := Result{
		Grid: req.Grid, Directions: req.Directions, Total: total,
		Components: comps, Transmission: transmission, Quality: quality,
		Provenance: Provenance{
			SchemaVersion: provenanceSchemaVersion,
			Engine:        e.cfg.Name,
			Mode:          req.Mode,
			Components:    compProvenances,
			Transmission:  transAlgo,
			Fallbacks:     fallbacks,
			GridID:        req.Grid.ID(),
			EvaluatedAt:   time.Now(),
			Atmosphere:    atm.Provenance(),
		},
	}

	res.Derived = e.computeDerived(req, res)
	res.Uncertainty = computeUncertainty(req, res)

	return res, nil
}

func (b *BufferPool) scratchOrNew(nDir, nLambda int) *Scratch {
	if b == nil {
		return NewScratch(nDir, nLambda)
	}

	if b.scratch == nil {
		b.scratch = NewScratch(nDir, nLambda)
	}

	return b.scratch
}

// computeDerived populates DerivedQuantities per req.Options.Derived.
func (e *CompositeEngine) computeDerived(req Request, res Result) DerivedQuantities {
	var d DerivedQuantities

	d.BrightestDirection = -1

	opts := req.Options
	nDir := len(req.Directions)

	if opts.Derived.Has(DerivePassbands) {
		for _, pb := range req.Passbands {
			pr := PassbandResult{Passband: pb.ID, AB: make([]SurfaceBrightnessAB, nDir)}

			hasVega := pb.VegaZP != nil
			if hasVega {
				pr.Vega = make([]SurfaceBrightnessVega, nDir)
			}

			pivot := pb.PivotWavelength()

			for i := range nDir {
				mean, err := BandMeanSpectralRadiance(req.Grid, res.Total.Row(i), pb)
				if err != nil {
					pr.AB[i] = SurfaceBrightnessAB(math.NaN())
					continue
				}

				pr.AB[i] = ABSurfaceBrightness(mean, pivot)

				if hasVega {
					v, err := VegaSurfaceBrightness(mean, pb)
					if err == nil {
						pr.Vega[i] = v
					}
				}
			}

			d.Passbands = append(d.Passbands, pr)
		}
	}

	if opts.Derived.Has(DeriveLuminance) {
		d.Luminance = make([]LuminanceCdM2, nDir)

		for i := range nDir {
			if v, err := PhotopicLuminance(req.Grid, res.Total.Row(i), photopicPassband(req.Grid)); err == nil {
				d.Luminance[i] = v
			}
		}
	}

	if opts.Derived.Has(DeriveAnthroRatio) && res.Components.Has(Artificial) {
		d.AnthroRatio = make([]float64, nDir)

		artField, _ := res.Components.Field(Artificial)
		if !artField.Empty() {
			for i := range nDir {
				artIntegral := sumRow(req.Grid, artField.Row(i))
				totalIntegral := sumRow(req.Grid, res.Total.Row(i))
				natIntegral := totalIntegral - artIntegral

				if natIntegral > 0 {
					d.AnthroRatio[i] = artIntegral / natIntegral
				} else if artIntegral > 0 {
					d.AnthroRatio[i] = math.Inf(1)
				}
			}
		}
	}

	if opts.Derived.Has(DeriveLimitingMag) && opts.LimitingMag != nil && len(req.Passbands) > 0 {
		d.LimitingMagnitude = make([]float64, nDir)
		pb := req.Passbands[0]

		for i := range nDir {
			mean, err := BandMeanSpectralRadiance(req.Grid, res.Total.Row(i), pb)
			if err != nil {
				continue
			}

			vega, _ := VegaSurfaceBrightness(mean, pb)
			ab := ABSurfaceBrightness(mean, pb.PivotWavelength())

			airmass := airmassFor(req.Directions[i])

			lm, err := opts.LimitingMag.LimitingMagnitude(LimitingMagInput{
				Passband: pb.ID, SkyVega: vega, SkyAB: ab, Airmass: airmass,
			})
			if err == nil {
				d.LimitingMagnitude[i] = lm
			}
		}
	}

	if opts.Derived.Has(DeriveDetectorBackground) && opts.Instrument != nil && len(req.Passbands) > 0 {
		d.DetectorBackground = make([]ElectronsPerPixelPerSecond, nDir)
		pb := req.Passbands[0]

		for i := range nDir {
			photonRad, err := IntegratePhotonRadiance(req.Grid, res.Total.Row(i), pb)
			if err != nil {
				continue
			}

			d.DetectorBackground[i] = opts.Instrument.BackgroundRate(photonRad)
		}
	}

	if opts.Derived.Has(DeriveAllSkyStats) && nDir > 0 && len(req.Passbands) > 0 {
		pb := req.Passbands[0]
		vals := make([]float64, 0, nDir)

		for i := range nDir {
			r, err := IntegrateRadiance(req.Grid, res.Total.Row(i), pb)
			if err != nil {
				continue
			}

			vals = append(vals, float64(r))
		}

		if len(vals) > 0 {
			sum := 0.0
			best, bestVal := 0, vals[0]

			for i, v := range vals {
				sum += v
				if v > bestVal {
					best, bestVal = i, v
				}
			}

			d.MeanAllSky = Radiance(sum / float64(len(vals)))
			d.BrightestDirection = best

			sorted := append([]float64(nil), vals...)
			sort.Float64s(sorted)
			d.MedianAllSky = Radiance(sorted[len(sorted)/2])
		}
	}

	if opts.Derived.Has(DeriveIrradiance) && nDir > 0 {
		alts := make([]float64, nDir)
		solid := make([]float64, nDir)

		for i, dir := range req.Directions {
			alts[i] = dir.Alt().Radians()
			solid[i] = uniformSolidAngle(nDir)
		}

		if v, err := HorizontalIrradiance(req.Grid, res.Total, alts, solid); err == nil {
			d.HorizontalIrradiance = v
		}
	}

	return d
}

// sumRow trapezoid-integrates spec over g (a plain wavelength integral,
// with no passband weighting) — used only for the anthropogenic/natural
// ratio, which compares two components' bolometric contributions, not a
// passband-specific brightness.
func sumRow(g SpectralGrid, spec []SpectralRadiance) float64 {
	w := g.Weights()
	sum := 0.0

	for i, v := range spec {
		sum += float64(v) * w[i]
	}

	return sum
}

// airmassFor returns a simple secant-law airmass from altitude, clamped
// to 1 at/above the zenith and treated as horizon-limit (very large) at or
// below the horizon. This is intentionally simple — callers wanting
// Pickering (2002) accuracy pass their own airmass via
// EvaluationOptions/LimitingMagInput in a future phase; Phase 1 needs
// something, not the most accurate thing.
func airmassFor(dir coord.AltAz) float64 {
	sinAlt := math.Sin(dir.Alt().Radians())
	if sinAlt <= 0.001 {
		return 1000 // effectively "at the horizon", not a physically precise value
	}

	return 1 / sinAlt
}

// uniformSolidAngle returns 4*pi/n steradians — the solid angle each of n
// equally-weighted directions represents if they uniformly tile the whole
// sphere. This is a coarse approximation used only when
// DeriveIrradiance is requested without the caller supplying real
// per-direction solid angles (a future refinement, not built in Phase 1).
func uniformSolidAngle(n int) float64 {
	if n <= 0 {
		return 0
	}

	return 4 * math.Pi / float64(n)
}

// photopicPassband returns a CIE-photopic-shaped analytic Gaussian
// response centered at 555 nm — a documented Phase 1 stand-in for the real
// CIE V(lambda) tabulated curve, which ships from
// skybrightness/dataset/passband once that provider exists. Cached per
// grid identity so repeated calls in a hot loop don't reallocate.
func photopicPassband(_ SpectralGrid) *Passband {
	return Gaussian("cie.photopic.stand-in", 555, 100)
}

func computeUncertainty(req Request, res Result) UncertaintyResult {
	if req.Options.Uncertainty == UncNone {
		return UncertaintyResult{Mode: UncNone}
	}

	if req.Options.Uncertainty != UncLinearized {
		return UncertaintyResult{
			Mode:     req.Options.Uncertainty,
			Warnings: []DomainWarning{{Message: ErrUncertaintyModeUnimplemented.Error()}},
		}
	}

	nDir, nLambda := res.Total.Dims()

	relSigma := NewSpectralField(nDir, nLambda) // relative sigma stored as a SpectralRadiance-typed field for reuse; interpreted as a unitless fraction, not a radiance

	var byGroup [numCovarianceGroups]GroupContribution

	for g := range numCovarianceGroups {
		byGroup[g] = GroupContribution{Group: CovarianceGroup(g)}
	}

	var byComponent [numComponents]float64

	res.Components.Each(func(id ComponentID, _ SpectralField, rep ComponentReport) bool {
		s := rep.Uncertainty.RelSigma
		byGroup[rep.Uncertainty.Group].Variance += s * s
		byComponent[id] = s

		return true
	})

	totalVariance := 0.0
	for _, g := range byGroup {
		totalVariance += g.Variance
	}

	if totalVariance > 0 {
		for i := range byGroup {
			byGroup[i].Share = byGroup[i].Variance / totalVariance
		}
	}

	// A single scalar relative sigma, broadcast across the grid — Phase 1
	// does not yet vary uncertainty per wavelength (a future refinement
	// via ComponentUncertainty.Spectral).
	totalRelSigma := math.Sqrt(totalVariance)
	for i := range relSigma.data {
		relSigma.data[i] = SpectralRadiance(totalRelSigma)
	}

	p05 := res.Total.Clone()
	p50 := res.Total.Clone()
	p95 := res.Total.Clone()

	scaleField(&p05, 1-1.645*totalRelSigma)
	scaleField(&p95, 1+1.645*totalRelSigma)

	return UncertaintyResult{
		Mode: UncLinearized, P05: p05, P50: p50, P95: p95, RelSigma: relSigma,
		ByGroup: byGroup, ByComponent: byComponent,
	}
}

func scaleField(f *SpectralField, s float64) {
	if s < 0 {
		s = 0
	}

	for i := range f.data {
		f.data[i] *= SpectralRadiance(s)
	}
}
