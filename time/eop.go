package time

import (
	"io"
	"log"
	"sync"

	"github.com/TuSKan/astrogo/time/internal/iers"
)

// EOP holds Earth Orientation Parameters (DUT1, polar motion, excess
// Length of Day) for a single epoch.
type EOP = iers.EOP

// Model provides Earth Orientation Parameters for a given Modified
// Julian Date. See [RegisterModel], [GetModel].
type Model = iers.Model

// ZeroModel is a Model that returns zero EOP for every epoch — the
// default until real data is registered via [RegisterModel] or found by
// the automatic lazy load a [Time.EOP]/[Time.UTC]/[Time.UT1] query
// triggers.
type ZeroModel = iers.ZeroModel

// Table is a parsed finals2000A-format EOP dataset. See [ParseFinals2000A].
type Table = iers.Table

// Sentinel errors for EOP lookups and downloads.
var (
	ErrOutOfRange = iers.ErrOutOfRange
	ErrNoRecords  = iers.ErrNoRecords
)

// EOPData is raw finals2000A content together with when that copy was
// written. See [EOPLoader].
type EOPData = iers.Data

// EOPLoader supplies raw EOP data to the lazy load that a
// [Time.EOP]/[Time.UTC]/[Time.UT1] query triggers.
//
// It exists so this package needs no knowledge of caches, HTTP, download
// consent or blob storage, and links none of it: importing astrogo/time
// to compute a Julian date used to cost about 17 MB of binary in
// cloud-storage and gRPC machinery that the arithmetic never touches.
// Importing astrogo/remote registers a loader automatically, which is
// what any program granting download consent already does, so nothing
// changes for a caller that wants downloads.
//
// [FileEOPLoader] serves the pre-seeded, no-dependencies case.
type EOPLoader = iers.Loader

// FileEOPLoader is an [EOPLoader] that reads one finals2000A file from a
// fixed path using nothing but the standard library — for a deployment
// that pre-seeds EOP data and wants no cloud-storage dependency. It
// downloads nothing.
type FileEOPLoader = iers.FileLoader

// Sentinel errors for the EOP loader path.
var (
	// ErrNoEOPLoader is returned when EOP data is needed and no
	// [EOPLoader] has been registered. The query degrades to zero EOP,
	// exactly as it does when download consent is absent.
	ErrNoEOPLoader = iers.ErrNoLoader

	// ErrNoEOPData is what an [EOPLoader] returns when it found nothing:
	// no pre-seeded file, or a download consent forbade. It is an
	// ordinary state, not a failure worth propagating.
	ErrNoEOPData = iers.ErrNoEOPData
)

// RegisterEOPLoader sets the process-wide [EOPLoader]. Passing nil
// unregisters. Importing astrogo/remote calls this for you.
func RegisterEOPLoader(l EOPLoader) { iers.RegisterLoader(l) }

// ParseFinals2000A parses a finals2000A-format IERS bulletin into a Table.
//
//nolint:wrapcheck // pure delegation to the unexported time/internal/iers, not a true external dependency
func ParseFinals2000A(r io.Reader) (*Table, error) { return iers.ParseFinals2000A(r) }

// RegisterModel sets the process-wide Earth orientation parameter model
// and marks the choice authoritative: the automatic lazy load a
// [Time.EOP]/[Time.UTC]/[Time.UT1] query would otherwise trigger (a
// pre-seeded on-disk cache file, then — if download consent was granted —
// a network fetch) will not run afterward, so it can never silently
// replace what was explicitly registered here.
//
// This matters most for RegisterModel(ZeroModel{}) — the natural way to
// ask for deterministic zero EOP — which previously WAS silently
// overridden the moment an uncovered lookup happened to find a
// finals2000A file already sitting in the cache directory, making "did
// this run use real or zero EOP" depend on ambient machine state rather
// than this call. See [EOPSource] to observe which source is active, and
// [ResetEOP] to undo this without pinning anything.
func RegisterModel(m Model) { iers.RegisterModel(m) }

// GetModel retrieves the process-wide Earth orientation parameter model.
// Defaults to ZeroModel until RegisterModel populates it, or a lazy load
// triggered by an EOP query succeeds.
func GetModel() Model { return iers.GetModel() }

// EOPSource reports where the currently active model came from:
// "zero" (the untouched default), "explicit" (a direct RegisterModel
// call), "cache" (the lazy loader read a pre-seeded finals2000A file from
// disk), or "network" (the lazy loader fetched one). Lets a caller — a
// test in particular — assert "this ran with real EOP" directly instead
// of inferring it from a lookup's numeric result.
func EOPSource() string { return iers.EOPSource() }

// ResetEOP restores the model to its pristine default state — ZeroModel,
// not explicit — discarding whatever RegisterModel or a lazy load
// previously set. Unlike RegisterModel(ZeroModel{}), this does not pin
// zero EOP: a later EOP query is still free to lazily load real data. Use
// this to start over; use RegisterModel(ZeroModel{}) to deliberately force
// zero EOP going forward.
func ResetEOP() { iers.Reset() }

// Coverage reports the currently-registered model's valid MJD range. ok
// is false if the model doesn't expose one (e.g. ZeroModel).
func Coverage() (mjdMin, mjdMax float64, ok bool) { return iers.Coverage() }

// SetRetryCooldown sets the minimum interval the automatic lazy load
// waits between fetch attempts after a failure (0 disables throttling).
// Default: 5 minutes.
func SetRetryCooldown(d Duration) { iers.SetRetryCooldown(d) }

var warnEOPUnavailableOnce sync.Once

// warnEOPUnavailable logs, once per process, that no real EOP data could
// be found for mjd — shared by Time.EOP() and the UT1<->UTC conversion's
// silent-degrade path (Time.UT1() itself still propagates the error
// instead of calling this).
func warnEOPUnavailable(mjd float64) {
	warnEOPUnavailableOnce.Do(func() {
		log.Printf("astrogo/time: IERS EOP data unavailable (MJD %.1f): using zero DUT1/polar motion/UT1-UTC. Topocentric accuracy degraded to ~1 arcsec; UT1 ≈ UTC (max error ≈ 0.9s). Call remote.EnableDownloads(..., remote.IERSFinals2000A) or pre-seed finals2000A.data for full accuracy.", mjd)
	})
}

// lookupEOP is the single place that attempts an automatic lazy load
// before looking up EOP for mjd: it checks whether the current model
// already covers mjd, then (if not) a pre-seeded on-disk cache file, then
// (if download consent was granted) a network fetch — see
// iers.EnsureLoaded. It never logs; callers decide whether to
// warn-and-degrade (Time.EOP, the UT1<->UTC conversion) or propagate the
// error (Time.UT1).
//
//nolint:wrapcheck // pure delegation to the unexported time/internal/iers, not a true external dependency
func lookupEOP(mjd float64) (EOP, error) {
	_ = iers.EnsureLoaded(mjd) // best-effort; the lookup below is authoritative

	return iers.GetModel().EOP(mjd)
}

// EOP returns Earth Orientation Parameters for t's epoch, first attempting
// an automatic lazy load if the registered model doesn't cover it (see
// [lookupEOP]/[iers.EnsureLoaded]), then degrading to a zero EOP and
// logging a one-time-per-process warning if that still doesn't help — the
// same fallback contract UT1<->UTC conversion uses internally. Never
// returns an error, for callers (like coord.NewContext) that can't
// themselves propagate a lookup failure.
func (t Time) EOP() EOP {
	mjd := t.MJD()

	eop, err := lookupEOP(mjd)
	if err != nil {
		warnEOPUnavailable(mjd)
	}

	return eop
}
