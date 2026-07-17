package time

import (
	"context"
	"io"
	"io/fs"
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
// default until real data is registered via [Fetch], [FetchIfStale], or
// [LoadFS].
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
// Defaults to ZeroModel until RegisterModel, Fetch, FetchIfStale, or
// LoadFS populates real data.
func GetModel() Model { return iers.GetModel() }

// Coverage reports the currently-registered model's valid MJD range. ok
// is false if the model doesn't expose one (e.g. ZeroModel).
func Coverage() (mjdMin, mjdMax float64, ok bool) { return iers.Coverage() }

// LoadFS parses a finals2000A-format file reached through any io/fs.FS —
// a local directory via os.DirFS, an embed.FS bundled by a downstream
// application, or any other stdlib-compatible filesystem — and registers
// it as the global EOP model.
//
//nolint:wrapcheck // pure delegation to the unexported time/internal/iers, not a true external dependency
func LoadFS(fsys fs.FS, name string) error { return iers.LoadFS(fsys, name) }

// Fetch downloads and registers fresh IERS EOP data immediately,
// bypassing the staleness/cooldown checks FetchIfStale applies. Calling
// it is itself the download consent for the remote.IERSFinals2000A
// endpoint (~3.7 MB); it still respects remote.SetOffline/Disable.
//
//nolint:wrapcheck // pure delegation to the unexported time/internal/iers, not a true external dependency
func Fetch(ctx context.Context) error { return iers.Fetch(ctx) }

// SetRetryCooldown sets the minimum interval FetchIfStale waits between
// fetch attempts after a failure (0 disables throttling). Default: 5 minutes.
func SetRetryCooldown(d stdtime.Duration) { iers.SetRetryCooldown(d) }

// FetchIfStale downloads fresh EOP data if the registered model doesn't
// cover t's epoch. Calling it is itself the download consent; it still
// respects remote.SetOffline/Disable.
//
//nolint:wrapcheck // pure delegation to the unexported time/internal/iers, not a true external dependency
func FetchIfStale(ctx context.Context, t Time) error {
	return iers.FetchIfStale(ctx, t.MJD())
}

//nolint:gochecknoglobals // one-time-per-process warning guard, mirrors warnUT1Once
var warnEOPOnce sync.Once

// EOP returns Earth Orientation Parameters for t's epoch from the
// process-wide registered Model, degrading to a zero EOP and logging a
// one-time-per-process warning if the epoch isn't covered — the same
// fallback contract UT1<->UTC conversion uses internally. Never returns
// an error, for callers (like coord.NewContext) that can't themselves
// propagate a lookup failure. Populate real data via Fetch, FetchIfStale,
// or LoadFS.
func (t Time) EOP() EOP {
	mjd := t.MJD()

	eop, err := iers.GetModel().EOP(mjd)
	if err != nil {
		warnEOPOnce.Do(func() {
			log.Printf("astrogo/time: IERS EOP data unavailable (MJD %.1f): using zero DUT1/polar motion. Topocentric accuracy degraded to ~1 arcsec.", mjd)
		})
	}

	return eop
}
