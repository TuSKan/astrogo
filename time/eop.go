package time

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/logging"
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
// It exists so this package needs no knowledge of caches, HTTP or download
// consent, and links none of it: importing astrogo/time to compute a Julian
// date used to cost about 17 MB of binary in cloud-storage and gRPC machinery
// that the arithmetic never touches. Blank-importing
// [github.com/TuSKan/astrogo/remote/eop] registers the loader that fetches
// through remote; importing remote alone does not (#294).
//
// [FSEOPLoader] serves the pre-seeded, no-dependencies case.
type EOPLoader = iers.Loader

// FSEOPLoader is an [EOPLoader] that reads one finals2000A object from a
// filesystem using nothing but the standard library — for a deployment that
// pre-seeds EOP data and wants no network dependency. It downloads nothing.
//
//	time.RegisterEOPLoader(time.FSEOPLoader{FS: os.DirFS(dir), Name: "finals2000A.data"})
//
// The filesystem is the caller's: os.DirFS for a directory on disk, or a
// remote.FS, which is an fs.FS. A missing object is [ErrNoEOPData], an
// ordinary state; any other read failure is returned as itself.
type FSEOPLoader = iers.FSLoader

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
// unregisters. Blank-importing [github.com/TuSKan/astrogo/remote/eop] calls
// this for you.
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

var (
	warnEOPUnavailableOnce sync.Once
	warnDeltaTHeldOnce     sync.Once
)

// warnEOPUnavailable logs, once per process, that no real EOP data could
// be found for mjd, and why: cause is the lookup's or the lazy load's error.
// Shared by every path that degrades: Time.EOP, the UT1<->UTC conversion,
// and Time.UT1 when nothing was loaded.
//
// An epoch past the bulletin's end is a different notice with a Once of its
// own: there DUT1 is not zero but held, and with one Once between them
// whichever case came first would silence the other for the process.
func warnEOPUnavailable(mjd float64, cause error) {
	if held, ok := errors.AsType[*deltaTHeldError](cause); ok {
		warnDeltaTHeldOnce.Do(func() { logDeltaTHeld(mjd, held) })

		return
	}

	warnEOPUnavailableOnce.Do(func() { logEOPUnavailable(mjd, cause) })
}

// logDeltaTHeld writes the notice for an epoch past the bulletin's end, apart
// from its Once for the reason [logEOPUnavailable] gives.
func logDeltaTHeld(mjd float64, held *deltaTHeldError) {
	logging.Warn("EOP bulletin ends before this epoch, holding ΔT at its last value",
		"mjd", mjd,
		"bulletin_last_mjd", held.lastMJD,
		"delta_t", fmt.Sprintf("%.3f s, TT − UT1 on the bulletin's last day", held.deltaT),
		"delta_t_sigma", fmt.Sprintf("%.1f s here, Huber (2000) from the bulletin's end", held.sigma),
		"polar_motion", "zero",
		"remedy", `a newer bulletin: import _ "github.com/TuSKan/astrogo/remote/eop", then `+
			"remote.EnableDownloads(0, remote.IERSFinals2000A)")
}

// logEOPUnavailable writes the warning, separately from the [sync.Once] that
// rations it.
//
// Split out so the message and its level can be tested. Behind the Once it
// fires exactly once per process, and whichever test happened to run first
// would spend it — so a test asserting the warning reaches an installed logger
// would pass or fail on test ordering rather than on the code.
func logEOPUnavailable(mjd float64, cause error) {
	// Warn, not Info, and so still emitted by the default logger: this is the
	// only notice a caller gets that the numbers changed. Time.EOP has no
	// error return, and the UT1<->UTC degrade path has already decided not to
	// fail. See the logging package.
	//
	// A short, fixed message with the detail in attributes, rather than three
	// sentences of prose: that is what slog is for, it keeps the line greppable
	// when a caller ships JSON, and the default text handler still prints every
	// attribute.
	// The remedy leads with the import because nothing else works without it:
	// since #294 no loader is registered until remote/eop is imported, and
	// EnableDownloads alone then consents to a fetch nothing will make.
	//
	// The 0.9 s is not a property of this code. It is the bound leap seconds
	// keep |UT1-UTC| inside, and CGPM Resolution 4 (2022) commits to abandoning
	// leap seconds by 2035 -- after which UT1-UTC grows without bound and this
	// figure goes stale. Levine, Tavella & Milton (2023) estimate that a
	// 1-minute tolerance would mean an adjustment roughly once a century, so
	// the degradation this warning describes gets slowly and permanently worse
	// from 2035 rather than staying at 0.9 s. See #147.
	//
	// The cause says which step failed: no loader registered, no cached
	// bulletin and no consent to fetch one, or a bulletin that does not reach
	// this epoch. It is what lets a caller tell the remedy apart from the rest.
	causeText := "unknown"
	if cause != nil {
		causeText = cause.Error()
	}

	logging.Warn("EOP unavailable, using zero DUT1 and polar motion",
		"mjd", mjd,
		"cause", causeText,
		"topocentric_error", "up to 13.5 arcsec, the Earth's rotation in 0.9 s",
		"ut1_error", "~0.9 s until leap seconds end in 2035, unbounded after",
		"remedy", `import _ "github.com/TuSKan/astrogo/remote/eop", then `+
			"remote.EnableDownloads(0, remote.IERSFinals2000A) or pre-seed finals2000A.data, "+
			"or RegisterModel(ZeroModel{}) to choose zero EOP deliberately")
}

// lookupEOP is the single place that attempts an automatic lazy load
// before looking up EOP for mjd: it checks whether the current model
// already covers mjd, then (if not) a pre-seeded on-disk cache file, then
// (if download consent was granted) a network fetch — see
// iers.EnsureLoaded. It never logs; callers decide whether to
// warn-and-degrade or propagate.
//
// It reports two failures separately, because they are not the same:
//
//   - err: the registered model could not answer for mjd, which only a
//     loaded bulletin that does not reach the epoch does. Time.UT1
//     propagates this one.
//   - defaulted: the answer is the untouched zero default, because nothing
//     was loaded and nothing was chosen; it carries the lazy load's error.
//     The zero model answers every epoch without error, so before #518 this
//     case looked like success and no caller warned. A caller that registers
//     ZeroModel deliberately is not defaulted: its source is "explicit".
//
//nolint:wrapcheck // pure delegation to the unexported time/internal/iers, not a true external dependency
func lookupEOP(mjd float64) (eop EOP, defaulted, err error) {
	loadErr := iers.EnsureLoaded(mjd)

	eop, err = iers.GetModel().EOP(mjd)
	if err != nil {
		if held, report, ok := heldPastBulletin(mjd); ok {
			return held, &report, nil
		}

		return eop, nil, err
	}

	if iers.EOPSource() == iers.SourceZero {
		if loadErr == nil {
			loadErr = errNothingLoaded
		}

		return eop, loadErr, nil
	}

	return eop, nil, nil
}

// errNothingLoaded is the cause given when the zero default answered and the
// lazy load reported no error of its own.
var errNothingLoaded = errors.New("time: no EOP bulletin loaded")

// heldPastBulletin is the EOP for an epoch past the loaded bulletin's last
// day: DUT1 such that ΔT = ΔAT + 32.184 s − DUT1 keeps the value it has on
// that day, and zero polar motion, with the report the notice is written
// from. ok is false inside or before the bulletin, or for a model that does
// not report its coverage, such as ZeroModel.
//
// # Why ΔT is held rather than extrapolated
//
// Past the bulletin nobody measures UT1, so every value is a forecast. This
// one used to be DUT1 = 0, UT1 pinned to UTC, which freezes ΔT at the
// leap-second count; nobody chose that, and it stops being even approximately
// true once leap seconds end, by 2035 under CGPM Resolution 4 (2022), after
// which UT1 − UTC grows without bound. The extrapolation on offer, Espenak &
// Meeus (2006), was already 6.2 s high in 2026 and climbs 0.6 s a year, while
// the bulletin shows ΔT flat since 2020 and published forecasts past 2030
// disagree in sign. So the held value is the measured one, continuous with
// the bulletin, and [DeltaTUncertainty] says what holding it costs (#696).
//
// A leap second registered past the bulletin's end moves ΔAT and DUT1
// together, so ΔT stays held across it.
func heldPastBulletin(mjd float64) (eop EOP, report deltaTHeldError, ok bool) {
	_, last, covered := iers.Coverage()
	if !covered || mjd <= last {
		return EOP{}, deltaTHeldError{}, false
	}

	// The bulletin's own last day; a model that cannot answer for it has
	// nothing to hold, and the lookup's out-of-range error stands.
	end, err := iers.GetModel().EOP(last)
	if err != nil {
		return EOP{}, deltaTHeldError{}, false
	}

	atEnd := deltaATAtMJD(last)

	return EOP{DUT1: end.DUT1 + deltaATAtMJD(mjd) - atEnd}, deltaTHeldError{
		lastMJD: last,
		deltaT:  atEnd + 32.184 - end.DUT1,
		sigma:   huberSigma((mjd - last) / 365.25),
	}, true
}

// deltaATAtMJD is TAI − UTC in seconds at a UTC Modified Julian Date.
func deltaATAtMJD(mjd float64) float64 {
	y, m, d, fd, _ := gofaext.JdToDate(2400000.5, mjd)

	return deltaAT(y, m, d, fd)
}

// deltaTHeldError is the lookup's report for an epoch past the bulletin's
// end. It is not a failure, which is why [lookupEOP] returns it as the
// defaulted cause rather than as err: Time.UT1 answers there, and the warning
// words it as the stated forecast it is.
type deltaTHeldError struct {
	lastMJD, deltaT, sigma float64
}

func (e *deltaTHeldError) Error() string {
	return fmt.Sprintf("time: past the EOP bulletin's last day, MJD %.1f; ΔT held at %.3f s (σ %.1f s)",
		e.lastMJD, e.deltaT, e.sigma)
}

// EOP returns Earth Orientation Parameters for t's epoch, first attempting
// an automatic lazy load if the registered model doesn't cover it (see
// [lookupEOP]/[iers.EnsureLoaded]), then degrading to a zero EOP and
// logging a one-time-per-process warning if that still doesn't help — the
// same fallback contract UT1<->UTC conversion uses internally. Never
// returns an error, for callers (like coord.NewContext) that can't
// themselves propagate a lookup failure.
//
// "Doesn't help" includes nothing having been loaded at all, the case of a
// program that never imported remote/eop: until #518 that one answered zeros
// without the warning. RegisterModel(ZeroModel{}) chooses zero EOP
// deliberately and stays silent.
//
// Past the end of a loaded bulletin, the answer is not zero: DUT1 is what
// holds ΔT = TT − UT1 at its value on the bulletin's last day, with zero
// polar motion, and the one-time notice says so with the held value and its
// uncertainty. See [DeltaT] for why it is held (#696).
func (t Time) EOP() EOP {
	mjd := t.MJD()

	eop, defaulted, err := lookupEOP(mjd)

	switch {
	case err != nil:
		warnEOPUnavailable(mjd, err)
	case defaulted != nil:
		warnEOPUnavailable(mjd, defaulted)
	}

	return eop
}
