package remote

import (
	"path/filepath"
	"testing"
)

func TestDefaultEndpointsHaveExplicitTimeouts(t *testing.T) {
	for _, ep := range defaultEndpoints() {
		switch ep.Kind {
		case KindAPI:
			if ep.Timeout == 0 {
				t.Errorf("%s: KindAPI endpoint has no explicit Timeout", ep.ID)
			}
		case KindFile:
			if ep.DownloadTimeout == 0 {
				t.Errorf("%s: KindFile endpoint has no explicit DownloadTimeout", ep.ID)
			}
		}
	}
}

func TestDefaultEndpointsCacheDirsMatchOnDiskLayout(t *testing.T) {
	t.Cleanup(func() {
		SetDataDir("")
		Reset()
	})

	SetDataDirPath(t.TempDir())

	want := map[EndpointID]string{
		IERSFinals2000A: "iers",
		NAIFSPK:         "jpl",
		NAIFLSK:         "jpl",
		OpenNGC:         "openngc",
	}

	for id, subsystem := range want {
		dir, err := CacheDir(id)
		if err != nil {
			t.Fatalf("CacheDir(%s): %v", id, err)
		}

		if got := filepath.Base(dir.LocalPath()); got != subsystem {
			t.Errorf("CacheDir(%s) = %s, want subsystem %q", id, dir, subsystem)
		}
	}
}
