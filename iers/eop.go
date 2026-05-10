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
)

// RegisterModel sets the globally used Earth orientation parameter model.
// This is typically called automatically by packages like `earth/iers` via init().
func RegisterModel(m Model) {
	modelMu.Lock()
	defer modelMu.Unlock()

	globalModel = m
}

// GetModel retrieves the globally used Earth orientation parameter model.
func GetModel() Model {
	modelMu.RLock()
	defer modelMu.RUnlock()

	return globalModel
}
