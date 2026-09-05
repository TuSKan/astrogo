// Package logging holds the one [slog.Logger] astrogo writes to.
//
// astrogo emits very little, and takes no logger parameter anywhere: a library
// that threads one through every signature makes every caller carry it, and
// almost none of them want to. One logger for the module, set here, like the
// endpoint registry and download consent in [github.com/TuSKan/astrogo/remote].
//
// # What astrogo logs
//
// Two kinds of line, deliberately not treated alike:
//
//   - [slog.LevelInfo] — progress a caller may want and does not need: a kernel
//     is downloading, an EOP table has loaded. Discarded by default.
//   - [slog.LevelWarn] — a result has silently degraded. Today there is one: no
//     Earth Orientation Parameters could be found for the epoch, so DUT1 and
//     polar motion are zero and topocentric accuracy has dropped to about an
//     arcsecond. [github.com/TuSKan/astrogo/time.Time.EOP] has no error return,
//     so that line is the only notice a caller gets. Emitted by default.
//
// So the default is quiet about progress and not quiet about accuracy:
//
//	logging.Set(slog.New(slog.DiscardHandler))   // silence everything
//	logging.Set(slog.New(h))                     // an Info handler: progress too
//	logging.Set(nil)                             // back to the default
//
// # Why a package of its own
//
// Because logging is cross-cutting and nothing else here is a natural owner.
// It was briefly internal, with remote re-exporting the setter — remote being
// where a caller already configures I/O, and every message astrogo emits being
// about data it fetched. That put the switch in the wrong place twice over: a
// caller using only time and coord had to import remote to quiet a warning
// time emits, and a package that is not below remote could never have used it.
//
// This package imports nothing from astrogo, so every layer can write to it —
// including time, which remote depends on and which therefore cannot depend on
// remote.
package logging

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
)

// current holds the active logger. Atomic rather than mutex-guarded: it is
// read on every log call and written approximately never.
var current atomic.Pointer[slog.Logger]

// Set installs l as the logger astrogo writes to, for the whole process.
// A nil l restores the default described in the package comment.
func Set(l *slog.Logger) { current.Store(l) }

// Logger returns the logger in force, never nil.
//
// Exported because astrogo's own packages are its callers, and because a
// program that wants to log alongside astrogo can reach the same destination
// without tracking it separately.
func Logger() *slog.Logger {
	if l := current.Load(); l != nil {
		return l
	}

	return defaultLogger
}

// defaultLogger is what astrogo writes to when a caller has set nothing.
//
// # Why not a discard handler
//
// Discarding everything is the tidy answer for a library, and it is wrong for
// one message here. Of the two kinds astrogo emits — see the package comment —
// the second is a warning that a result silently degraded, and Time.EOP has no
// error return to carry it instead. Discarding by default would remove the only
// signal that the numbers changed, which is the opposite of what quieting a
// library is for.
//
// So the default passes Warn and above and drops the rest. A caller who wants
// silence, or wants the progress lines, says so in one call.
var defaultLogger = slog.New(newWarnOnlyHandler())

// newWarnOnlyHandler builds the default handler: Warn and above to stderr, in
// the plain one-line form the standard log package produced before this
// existed, so quieting the informational lines is the only visible change.
func newWarnOnlyHandler() slog.Handler {
	return &warnOnlyHandler{
		inner: slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}),
	}
}

// warnOnlyHandler drops anything below Warn without formatting it.
type warnOnlyHandler struct{ inner slog.Handler }

func (h *warnOnlyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn
}

func (h *warnOnlyHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.inner.Handle(ctx, r) //nolint:wrapcheck // a pass-through to the wrapped handler
}

func (h *warnOnlyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &warnOnlyHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *warnOnlyHandler) WithGroup(name string) slog.Handler {
	return &warnOnlyHandler{inner: h.inner.WithGroup(name)}
}
