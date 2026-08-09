package skybrightness

// Scratch is per-goroutine reusable evaluation state: a transmission
// buffer and a spare SpectralField sized for one component's contribution.
// A Scratch is never shared between goroutines and is never returned to a
// caller from Evaluate/EvaluateBatch — construct one per goroutine (see
// EvaluateBatch's use of internal/parallel.MapChunked, which calls
// NewScratch exactly once per worker goroutine, not once per direction).
type Scratch struct {
	transmission []Transmission
	component    SpectralField
	quad         []float64
}

// NewScratch allocates a Scratch sized for nDir directions x nLambda
// wavelengths.
func NewScratch(nDir, nLambda int) *Scratch {
	return &Scratch{
		transmission: make([]Transmission, nDir*nLambda),
		component:    NewSpectralField(nDir, nLambda),
		quad:         make([]float64, nLambda),
	}
}

// Transmission returns the scratch's transmission buffer, sized
// nDir*nLambda, direction-major (matching SpectralField's own layout).
func (s *Scratch) Transmission() []Transmission { return s.transmission }

// Component returns the scratch's spare per-component field, zeroed by the
// caller before each use.
func (s *Scratch) Component() SpectralField { return s.component }
