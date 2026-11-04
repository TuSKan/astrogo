package iers

import (
	"sync"
	"sync/atomic"
)

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

// Values EOPSource returns, naming where the currently active model came
// from — see EOPSource's own doc comment.
const (
	SourceZero     = "zero"
	SourceExplicit = "explicit"
	SourceCache    = "cache"
	SourceNetwork  = "network"
)

var (
	// modelMu orders the writers of current. Readers take no lock: every
	// Earth orientation lookup in a process reads the model, from every core,
	// and the read locks a lookup took were each an atomic add on one shared
	// counter, so sixteen goroutines looked up less than one did (#953).
	modelMu sync.Mutex

	// current is the registered model state, nil until the first
	// registration and after Reset: see state.
	current atomic.Pointer[modelState]

	// pristine is the state before any registration.
	pristine = modelState{model: ZeroModel{}, source: SourceZero}
)

// modelState is what a lookup reads, as one value, so that the model, its
// source and whether a caller chose it all come from one registration.
type modelState struct {
	model Model
	// source records provenance for EOPSource — purely observational,
	// never consulted by any decision logic in this package.
	source string
	// explicit is true once a caller has called RegisterModel directly —
	// see RegisterModel and EnsureLoaded (fetch.go) for what this gates.
	explicit bool
}

// state is the current model state, without a lock.
func state() *modelState {
	if s := current.Load(); s != nil {
		return s
	}

	return &pristine
}

// RegisterModel sets the globally used Earth orientation parameter model,
// and marks the choice authoritative: EnsureLoaded's lazy loader (an
// on-disk cache probe, then — if download consent was granted — a network
// fetch) will not run again after this call, so it can never silently
// replace what was explicitly registered.
//
// This matters most for RegisterModel(ZeroModel{}): before this guard
// existed, that call — the natural way to ask for deterministic zero EOP —
// was itself overridden the moment an uncovered lookup found a
// finals2000A file sitting in the cache directory, which made "did this
// run use real or zero EOP" depend on ambient machine state rather than
// the caller's own choice. See EOPSource to observe which case is active.
//
// Use registerModelInternal for the lazy loader's own opportunistic
// registration — it must NOT mark the model explicit, or a caller's later
// RegisterModel call would have nothing left to override.
func RegisterModel(m Model) {
	modelMu.Lock()
	defer modelMu.Unlock()

	current.Store(&modelState{model: m, source: SourceExplicit, explicit: true})
}

// registerModelInternal installs m without marking the choice explicit —
// EnsureLoaded's own lazy-load path (fetch.go) uses this so a caller's own
// RegisterModel call, past or future, remains authoritative. See
// RegisterModel's doc comment. source records provenance for EOPSource
// (SourceCache or SourceNetwork).
func registerModelInternal(m Model, source string) {
	modelMu.Lock()
	defer modelMu.Unlock()

	if state().explicit {
		return
	}

	current.Store(&modelState{model: m, source: source})
}

// Reset restores the model to its pristine default state — ZeroModel, not
// explicit, source SourceZero — discarding whatever was previously
// registered or lazily loaded. Unlike RegisterModel(ZeroModel{}), this does
// NOT mark the result explicit, so a subsequent EnsureLoaded is free to
// lazily load real data again; use it to start over, not to pin zero EOP
// (for that, call RegisterModel(ZeroModel{}) instead). Mirrors remote.Reset's
// role in that package: primarily for tests, but not test-only — any
// caller that wants a clean slate can use it.
func Reset() {
	// Before taking modelMu: starting over means the next lookup may read
	// the cached bulletin again, which the read cooldown would otherwise
	// hold closed.
	forgetCacheRead()

	modelMu.Lock()
	defer modelMu.Unlock()

	current.Store(nil)
}

// modelIsExplicit reports whether RegisterModel has been called directly,
// as opposed to internally by the lazy loader — see EnsureLoaded, which
// skips its own disk-probe/network-fetch dance entirely once this is true.
func modelIsExplicit() bool {
	return state().explicit
}

// EOPSource reports where the currently active model came from: SourceZero
// (the untouched default, ZeroModel), SourceExplicit (a direct RegisterModel
// call — including RegisterModel(ZeroModel{})), SourceCache (the lazy
// loader read a pre-seeded finals2000A file from disk), or SourceNetwork
// (the lazy loader fetched one). Exists so a test — or a caller who cares —
// can assert "this ran with real EOP" instead of inferring it indirectly
// from a lookup's numeric result.
func EOPSource() string {
	return state().source
}

// GetModel retrieves the globally used Earth orientation parameter model.
// Defaults to ZeroModel until RegisterModel populates it directly, or a
// lazy load triggered by EnsureLoaded succeeds.
func GetModel() Model {
	return state().model
}

// Lookup is the current model's EOP at mjd when nothing need be loaded to give
// it, which is EnsureLoaded's own first answer: a model a caller registered is
// authoritative, and a loaded table that covers mjd needs no other. It reads
// the model state once and interpolates once, where a lazily loaded table's
// covered epoch was interpolated twice, once by EnsureLoaded to learn it was
// covered and once for the answer (#953). ok is false otherwise,
// including when the model does not answer mjd, and the caller then goes
// through EnsureLoaded as before.
func Lookup(mjd float64) (eop EOP, ok bool) {
	s := state()

	if !s.explicit {
		if _, table := s.model.(*Table); !table || s.source == SourceZero {
			return EOP{}, false
		}
	}

	eop, err := s.model.EOP(mjd)

	return eop, err == nil
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
