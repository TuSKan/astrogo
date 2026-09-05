package logging_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/logging"
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

	got := logging.Get()

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

	logging.Get().Info("downloading", "bytes", 42)
	logging.Get().Warn("degraded", "mjd", 60000.0)

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

	if !logging.Get().Enabled(t.Context(), slog.LevelInfo) {
		t.Fatal("precondition: the installed logger should pass Info")
	}

	logging.Set(nil)

	if logging.Get().Enabled(t.Context(), slog.LevelInfo) {
		t.Error("Set(nil) did not restore the default, which drops Info")
	}
}

// TestSilenceIsAvailable pins that a caller who wants nothing at all can have
// it — the behaviour #108 asked for as the default, still reachable in one
// call.
func TestSilenceIsAvailable(t *testing.T) {
	defer logging.Set(nil)

	logging.Set(slog.New(slog.DiscardHandler))

	if logging.Get().Enabled(t.Context(), slog.LevelError) {
		t.Error("a discard logger still reports Error as enabled; a caller " +
			"cannot fully silence the library")
	}
}
