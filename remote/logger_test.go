package remote_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/logging"
	"github.com/TuSKan/astrogo/remote"
)

// TestSetLoggerReachesTheSharedLogger pins the re-export, which is the part of
// this design most likely to break silently.
//
// The logger cannot live in remote, because remote imports astrogo/time and
// time is where the message that matters most is written — so the state sits
// in internal/logging, below both, and remote re-exports the setter. That
// indirection is invisible at the call site: if SetLogger ever stopped writing
// to the shared state, remote's own download line would still work while
// time's EOP warning quietly went to the old default.
func TestSetLoggerReachesTheSharedLogger(t *testing.T) {
	// Not parallel: it swaps process-wide state.
	defer remote.SetLogger(nil)

	var buf bytes.Buffer

	want := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	remote.SetLogger(want)

	if got := logging.Get(); got != want {
		t.Fatal("remote.SetLogger did not install the logger every astrogo " +
			"package reads; time's EOP warning would still go to the default")
	}

	// And a write through the shared accessor lands in the buffer, so the
	// pointer comparison above is not the whole of it.
	logging.Get().Warn("degraded", "mjd", 60000.0)

	if out := buf.String(); !strings.Contains(out, "degraded") || !strings.Contains(out, "mjd=60000") {
		t.Errorf("the installed logger did not receive the record:\n%s", out)
	}
}

// TestSetLoggerNilRestoresTheDefault pins the documented way back.
//
// Without it a caller who redirected logging for one operation could not undo
// it, and any test that redirected would leak its buffer into the rest of the
// process.
func TestSetLoggerNilRestoresTheDefault(t *testing.T) {
	defer remote.SetLogger(nil)

	var buf bytes.Buffer

	remote.SetLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	remote.SetLogger(nil)

	logging.Get().Warn("after reset")

	if buf.Len() != 0 {
		t.Errorf("a warning still reached the replaced logger after "+
			"SetLogger(nil):\n%s", buf.String())
	}

	// The default is back, which means Warn passes and Info does not.
	if logging.Get().Enabled(t.Context(), slog.LevelInfo) {
		t.Error("after SetLogger(nil) the logger passes Info; the default should not")
	}

	if !logging.Get().Enabled(t.Context(), slog.LevelWarn) {
		t.Error("after SetLogger(nil) the logger drops Warn; the default should not")
	}
}
