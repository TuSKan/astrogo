package epoch_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/internal/remotetest"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/logging"
	"github.com/TuSKan/astrogo/remote"
)

// TestNothingLoadedWarnsAndChosenZeroDoesNot is #518: with no bulletin loaded
// and none chosen, EOP degrades to zero, and before #518 it did so in
// silence. The zero model answers every epoch without error, and every
// caller warned only on an error, so the warning fired only when a loaded
// bulletin missed the epoch. A program without remote/eop lost an arcsecond
// of topocentric accuracy and up to 0.9 s of UT1 with no notice at all.
//
// Now each degrading path warns, and Time.UT1 still returns no error there;
// RegisterModel(ZeroModel{}), the documented way to choose zero EOP, is
// silent.
func TestNothingLoadedWarnsAndChosenZeroDoesNot(t *testing.T) {
	// Not parallel: the EOP model, the logger and the warning's Once are all
	// process-wide.

	// An empty cache, so the lazy load finds no bulletin to read, and a policy
	// refusing the download, so it fetches none.
	remotetest.RefuseDownloads(t, remote.IERSFinals2000A)
	t.Cleanup(remote.Capture().Restore)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	epoch.ResetEOP()
	t.Cleanup(epoch.ResetEOP)

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { logging.Set(nil) })

	tm := epoch.FromJD(2461318.5, epoch.UTC)

	degrading := []struct {
		name string
		call func() error
	}{
		{"Time.EOP", func() error {
			if eop := tm.EOP(); eop != (epoch.EOP{}) {
				t.Errorf("Time.EOP with nothing loaded: %+v, want zero", eop)
			}

			return nil
		}},
		{"Time.UT1", func() error {
			if _, err := tm.UT1(); err != nil {
				return fmt.Errorf("Time.UT1: %w", err)
			}

			return nil
		}},
		{"UT1 to UTC", func() error {
			epoch.FromJD(2461318.5, epoch.UT1).UTC()

			return nil
		}},
	}

	for _, d := range degrading {
		epoch.ResetEOPWarning()
		buf.Reset()

		if err := d.call(); err != nil {
			t.Errorf("%s with nothing loaded returned %v; it degrades to DUT1 = 0 instead", d.name, err)
		}

		out := buf.String()
		if !strings.Contains(out, "EOP unavailable") || !strings.Contains(out, "level=WARN") {
			t.Errorf("%s degraded to zero EOP with nothing loaded and did not warn:\n%q", d.name, out)
		}

		if !strings.Contains(out, "cause=") {
			t.Errorf("%s warned without saying why:\n%q", d.name, out)
		}
	}

	// Chosen deliberately: no warning on any path.
	epoch.RegisterModel(epoch.ZeroModel{})

	for _, d := range degrading {
		epoch.ResetEOPWarning()
		buf.Reset()

		if err := d.call(); err != nil {
			t.Errorf("%s with ZeroModel registered returned %v", d.name, err)
		}

		if buf.Len() != 0 {
			t.Errorf("%s warned although ZeroModel was registered deliberately:\n%q", d.name, buf.String())
		}
	}
}
