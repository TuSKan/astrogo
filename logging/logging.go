// Package logging holds the one [slog.Logger] astrogo writes to.
//
// It is the only package in astrogo that imports [log/slog]: everything else
// calls [Info], [Warn] and their context variants, which name no slog type. A
// caller configuring the destination needs slog to build a logger, and nothing
// else does.
//
// astrogo takes no logger parameter anywhere. A library that threads one
// through every signature makes every caller carry it, and almost none of them
// want to; one logger for the module is set here, like the endpoint registry
// and download consent in [github.com/TuSKan/astrogo/remote].
//
// # What astrogo logs
//
// Two levels, deliberately not treated alike:
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
	"runtime"
	"sync/atomic"
	"time"
)

// current holds the active logger. Atomic rather than mutex-guarded: it is
// read on every log call and written approximately never.
var current atomic.Pointer[slog.Logger]

// Set installs l as the logger astrogo writes to, for the whole process.
// A nil l restores the default described in the package comment.
func Set(l *slog.Logger) { current.Store(l) }

// Logger returns the logger in force, never nil.
//
// [Info] and [Warn] are the normal way in; this is for a level they do not
// cover, or for a program that wants to log to the same destination astrogo
// does without tracking it separately.
func Logger() *slog.Logger {
	if l := current.Load(); l != nil {
		return l
	}

	return defaultLogger
}

// Info records progress: something took time, or a resource was loaded.
// Discarded by the default logger.
func Info(msg string, args ...any) { emit(context.Background(), slog.LevelInfo, msg, args...) }

// InfoContext is [Info] with a context, so a handler can pick up trace
// identifiers from it. Prefer it wherever a context is already in hand.
func InfoContext(ctx context.Context, msg string, args ...any) {
	emit(ctx, slog.LevelInfo, msg, args...)
}

// Warn records that a result silently degraded — the caller got an answer, and
// it is less accurate than it looks. Emitted by the default logger.
func Warn(msg string, args ...any) { emit(context.Background(), slog.LevelWarn, msg, args...) }

// WarnContext is [Warn] with a context.
func WarnContext(ctx context.Context, msg string, args ...any) {
	emit(ctx, slog.LevelWarn, msg, args...)
}

// emit builds the record these wrappers share.
//
// It goes through [slog.Handler] rather than calling l.Info/l.Warn, so that the
// source position a handler records with AddSource is the astrogo line that
// logged, not this file. slog's own top-level functions do the same thing for
// the same reason: a wrapper that forwards to the logger's convenience methods
// makes every record point at the wrapper.
func emit(ctx context.Context, level slog.Level, msg string, args ...any) {
	l := Logger()
	if !l.Enabled(ctx, level) {
		return // the common case for Info under the default logger
	}

	// Skip runtime.Callers, emit, and the exported wrapper that called it.
	var pcs [1]uintptr

	runtime.Callers(3, pcs[:])

	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)

	// The handler's error is discarded deliberately: a logger that cannot log
	// has nowhere to report that, and astrogo will not fail an ephemeris
	// lookup because stderr was closed.
	_ = l.Handler().Handle(ctx, r)
}

// defaultLogger is what astrogo writes to when a caller has set nothing.
//
// # Why not a discard handler
//
// Discarding everything is the tidy answer for a library, and it is wrong for
// one message here. Of the two levels astrogo emits — see the package comment —
// Warn means a result silently degraded, and Time.EOP has no error return to
// carry that instead. Discarding by default would remove the only signal that
// the numbers changed, which is the opposite of what quieting a library is for.
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
