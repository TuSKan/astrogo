package iers

import "sync"

// EOP holds Earth Orientation Parameters for a single epoch.
// All fields follow IERS conventions.
type EOP struct {
	// DUT1 is UT1 - UTC in seconds.
	DUT1 float64
	// XP is the x-component of polar motion in radians.
	XP float64
	// YP is the y-component of polar motion in radians.
	YP float64
	// LOD is the excess Length of Day in seconds.
	LOD float64
}

// Model is the interface for providing Earth orientation parameters.
type Model interface {
	// EOP returns Earth Orientation Parameters for the given Modified Julian Date.
	EOP(mjd float64) (EOP, error)
}

// ZeroModel is a Model that returns zero EOP for all epochs.
// It is suitable for applications where sub-arcsecond accuracy is not required.
type ZeroModel struct{}

// EOP returns an all-zero EOP record.
func (ZeroModel) EOP(_ float64) (EOP, error) {
	return EOP{}, nil
}

//nolint:gochecknoglobals // singleton EOP model with RWMutex guard
var (
	modelMu     sync.RWMutex
	globalModel Model = ZeroModel{}
	loadOnce    sync.Once
)

// RegisterModel sets the globally used Earth orientation parameter model.
// Calling it before the first GetModel/FetchIfStale/FetchNow query
// pre-empts iers's own lazy load of the embedded finals2000A snapshot —
// see loadEmbedded.
func RegisterModel(m Model) {
	modelMu.Lock()
	defer modelMu.Unlock()

	globalModel = m
}

// registerIfDefault installs m only if no model has been explicitly
// registered yet (globalModel is still the zero-value default). Used by
// loadEmbedded so an explicit RegisterModel/LoadFS call always wins over
// the lazily-loaded embedded snapshot, regardless of call order.
func registerIfDefault(m Model) {
	modelMu.Lock()
	defer modelMu.Unlock()

	if _, isDefault := globalModel.(ZeroModel); isDefault {
		globalModel = m
	}
}

// GetModel retrieves the globally used Earth orientation parameter model.
// The first call triggers a one-time lazy load of the embedded
// finals2000A snapshot (see loadEmbedded) unless a model was already
// registered explicitly.
func GetModel() Model {
	loadOnce.Do(loadEmbedded)

	modelMu.RLock()
	defer modelMu.RUnlock()

	return globalModel
}

// coverer is implemented by Model values that know their own valid MJD
// range (currently only *Table, built from a parsed finals2000A.all file).
type coverer interface {
	Coverage() (mjdMin, mjdMax float64)
}

// Coverage reports the currently-registered global Model's valid MJD range.
// ok is false if the registered model doesn't expose a coverage range (e.g.
// ZeroModel, or a custom Model that hasn't opted in) — such a model can be
// queried for any epoch without ErrOutOfRange, but its accuracy is not
// epoch-dependent either, so there is nothing to report.
//
// Use this to proactively check whether the currently-registered EOP data
// still covers the epoch you are about to compute with — e.g. at service
// startup, or on a periodic health check — rather than relying on the
// one-time degradation warning coord.NewContext and time.Time log
// internally the first time a query falls outside the model's range.
func Coverage() (mjdMin, mjdMax float64, ok bool) {
	c, ok := GetModel().(coverer)
	if !ok {
		return 0, 0, false
	}

	mjdMin, mjdMax = c.Coverage()

	return mjdMin, mjdMax, true
}
