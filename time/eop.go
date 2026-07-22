package time

import (
	"io"
	"log"
	"sync"
	stdtime "time"

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
	ErrOutOfRange    = iers.ErrOutOfRange
	ErrNoRecords     = iers.ErrNoRecords
	ErrEOPHTTPStatus = iers.ErrEOPHTTPStatus
)

// ParseFinals2000A parses a finals2000A-format IERS bulletin into a Table.
//
//nolint:wrapcheck // pure delegation to the unexported time/internal/iers, not a true external dependency
func ParseFinals2000A(r io.Reader) (*Table, error) { return iers.ParseFinals2000A(r) }

// RegisterModel sets the process-wide Earth orientation parameter model.
func RegisterModel(m Model) { iers.RegisterModel(m) }

// GetModel retrieves the process-wide Earth orientation parameter model.
// Defaults to ZeroModel until RegisterModel populates it, or a lazy load
// triggered by an EOP query succeeds.
func GetModel() Model { return iers.GetModel() }

// Coverage reports the currently-registered model's valid MJD range. ok
// is false if the model doesn't expose one (e.g. ZeroModel).
func Coverage() (mjdMin, mjdMax float64, ok bool) { return iers.Coverage() }

// SetRetryCooldown sets the minimum interval the automatic lazy load
// waits between fetch attempts after a failure (0 disables throttling).
// Default: 5 minutes.
func SetRetryCooldown(d stdtime.Duration) { iers.SetRetryCooldown(d) }

//nolint:gochecknoglobals // one-time-per-process warning guard
var warnEOPUnavailableOnce sync.Once

// warnEOPUnavailable logs, once per process, that no real EOP data could
// be found for mjd — shared by Time.EOP() and the UT1<->UTC conversion's
// silent-degrade path (Time.UT1() itself still propagates the error
// instead of calling this).
func warnEOPUnavailable(mjd float64) {
	warnEOPUnavailableOnce.Do(func() {
		log.Printf("astrogo/time: IERS EOP data unavailable (MJD %.1f): using zero DUT1/polar motion/UT1-UTC. Topocentric accuracy degraded to ~1 arcsec; UT1 ≈ UTC (max error ≈ 0.9s). Call remote.EnableDownloads(remote.IERSFinals2000A, ...) or pre-seed finals2000A.data for full accuracy.", mjd)
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
