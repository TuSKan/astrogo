// Package logging holds the one [slog.Logger] astrogo writes to, so that
// packages on different layers can share it without importing each other.
//
// # Why this is not simply in remote
//
// The obvious home is remote, which owns I/O policy and is where a caller
// already goes to grant download consent — and remote.SetLogger is indeed the
// public way to set it. But remote imports astrogo/time, so time cannot import
// remote back, and time is where the message that matters most is written: the
// warning that Earth Orientation Parameters were unavailable and results have
// silently degraded.
//
// So the state lives here, below both, and remote re-exports the setter. There
// is still exactly one public entry point.
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

// Set installs l as the logger astrogo writes to. A nil l restores the
// default. See [Default] for what that default does.
func Set(l *slog.Logger) {
	if l == nil {
		current.Store(nil)

		return
	}

	current.Store(l)
}

// Get returns the logger in force, never nil.
func Get() *slog.Logger {
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
// one message here. astrogo emits two kinds of line:
//
//   - Informational progress — a kernel is downloading, an EOP table loaded.
//     A library has no business putting these on someone's stderr, and they
//     are discarded unless a caller asks for them.
//   - A warning that a result silently degraded — EOP data could not be found,
//     so DUT1 and polar motion are zero and topocentric accuracy has dropped
//     to about an arcsecond. Time.EOP has no error return, so this line is the
//     *only* notice a caller gets.
//
// Discarding the second by default would remove the sole signal that the
// numbers changed, which is the opposite of what quieting a library is for. So
// the default passes Warn and above and drops the rest, and a caller who wants
// silence, or wants the progress lines, sets their own logger.
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
