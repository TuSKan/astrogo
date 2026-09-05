package logging_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/logging"
)

// TestDefaultKeepsWarningsAndDropsProgress pins the one decision in this
// package.
//
// #108 proposed defaulting to a discard handler, which is the tidy answer for
// a library and is wrong for one message here. astrogo emits two kinds of
// line:
//
//   - progress, which a library has no business putting on someone's stderr
//   - a warning that a result silently degraded, which is the *only* notice a
//     caller gets that EOP were unavailable and topocentric accuracy dropped
//     to about an arcsecond, because time.Time.EOP has no error return
//
// Discarding by default would have quieted the second along with the first,
// removing the sole signal that the numbers changed. So the default is
// level-based, and this is what says so.
func TestDefaultKeepsWarningsAndDropsProgress(t *testing.T) {
	// Not parallel: it swaps the process-wide logger.
	defer logging.Set(nil)

	logging.Set(nil) // start from the default

	got := logging.Logger()

	if got == nil {
		t.Fatal("Get returned nil, which it never may")
	}

	if got.Enabled(t.Context(), slog.LevelInfo) {
		t.Error("the default logger passes Info. Progress lines would land on " +
			"a caller's stderr uninvited, which is what #108 was about.")
	}

	if got.Enabled(t.Context(), slog.LevelDebug) {
		t.Error("the default logger passes Debug")
	}

	if !got.Enabled(t.Context(), slog.LevelWarn) {
		t.Error("the default logger drops Warn. The EOP-unavailable warning is " +
			"the only notice a caller gets that accuracy silently degraded; " +
			"quieting it is the opposite of what quieting a library is for.")
	}

	if !got.Enabled(t.Context(), slog.LevelError) {
		t.Error("the default logger drops Error")
	}
}

// TestSetRedirectsEverything checks that a caller who asks for the progress
// lines gets them, and that they go where they were sent.
func TestSetRedirectsEverything(t *testing.T) {
	defer logging.Set(nil)

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	logging.Logger().Info("downloading", "bytes", 42)
	logging.Logger().Warn("degraded", "mjd", 60000.0)

	out := buf.String()

	for _, want := range []string{"downloading", "bytes=42", "degraded", "mjd=60000"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

// TestSetNilRestoresTheDefault pins the documented way back, so a test or a
// caller that redirected can undo it.
func TestSetNilRestoresTheDefault(t *testing.T) {
	defer logging.Set(nil)

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	if !logging.Logger().Enabled(t.Context(), slog.LevelInfo) {
		t.Fatal("precondition: the installed logger should pass Info")
	}

	logging.Set(nil)

	if logging.Logger().Enabled(t.Context(), slog.LevelInfo) {
		t.Error("Set(nil) did not restore the default, which drops Info")
	}
}

// TestSilenceIsAvailable pins that a caller who wants nothing at all can have
// it — the behaviour #108 asked for as the default, still reachable in one
// call.
func TestSilenceIsAvailable(t *testing.T) {
	defer logging.Set(nil)

	logging.Set(slog.New(slog.DiscardHandler))

	if logging.Logger().Enabled(t.Context(), slog.LevelError) {
		t.Error("a discard logger still reports Error as enabled; a caller " +
			"cannot fully silence the library")
	}
}

// TestSourceIsTheCallerNotThisPackage pins the reason [logging.Info] and
// friends build a record by hand instead of forwarding to l.Info.
//
// A wrapper that calls the logger's convenience method makes every record's
// source position point at the wrapper. Under the default handler that is
// invisible, because it does not enable AddSource — so a caller who turns
// AddSource on would find every astrogo line attributed to logging.go, and
// nothing else would fail.
//
// slog's own top-level functions take the same trouble for the same reason.
func TestSourceIsTheCallerNotThisPackage(t *testing.T) {
	defer logging.Set(nil)

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
		AddSource: true,
	})))

	logging.Info("from the test")

	out := buf.String()

	if !strings.Contains(out, "logging_test.go") {
		t.Errorf("the record's source is not this test file:\n%s\n"+
			"  Info must build the record itself so the caller's line survives.", out)
	}

	if strings.Contains(out, "source=") && strings.Contains(out, "/logging.go:") {
		t.Errorf("the record's source points into the logging package:\n%s", out)
	}
}

// TestInfoIsSkippedCheaplyWhenDisabled pins the guard that keeps the discarded
// case from formatting anything.
//
// Info is dropped by the default logger, and it sits on the download path where
// it would otherwise build a record and walk the stack for every fetch.
func TestInfoIsSkippedCheaplyWhenDisabled(t *testing.T) {
	defer logging.Set(nil)

	logging.Set(nil)

	var buf bytes.Buffer

	// A handler that would panic if it were ever handed a record.
	logging.Set(slog.New(refusingHandler{}))
	logging.Info("must not reach the handler")

	logging.Set(nil)

	if buf.Len() != 0 {
		t.Error("unexpected output")
	}
}

// refusingHandler fails the test if anything reaches it, while reporting every
// level as disabled.
type refusingHandler struct{}

func (refusingHandler) Enabled(context.Context, slog.Level) bool { return false }
func (refusingHandler) Handle(context.Context, slog.Record) error {
	panic("a disabled handler was handed a record")
}
func (h refusingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h refusingHandler) WithGroup(string) slog.Handler      { return h }
