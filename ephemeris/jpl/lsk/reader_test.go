package lsk_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/time"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestLSKReader(t *testing.T) {
	prov, err := jpl.NewProvider(core.Planets, "de440s")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Cleanup(func() {
		err := prov.Close()
		if err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	lskPath := filepath.Join(prov.DataDir, "lsk", "naif0012.tls")

	f, err := os.Open(lskPath)
	testutil.AssertNoError(t, err)

	t.Cleanup(func() {
		err := f.Close()
		if err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	})

	r, err := lsk.NewReader(f)
	testutil.AssertNoError(t, err)

	t.Cleanup(func() {
		err := r.Close()
		if err != nil {
			t.Errorf("failed to close reader: %v", err)
		}
	})

	// Test UTC to TDB conversion
	// Difference between UTC and TDB is roughly 64 seconds + periodic terms at J2000
	j2000 := time.FromJD(2451545.0, time.UTC)
	tdbJD := lsk.UTCToTDB(j2000, r)

	diffSeconds := (tdbJD - j2000.JD()) * 86400.0
	// TDB is ahead of UTC by ~64.184 seconds at J2000
	testutil.AssertNear(t, "TDB-UTC at J2000", diffSeconds, 64.184, 1.0)
}
