package crosssection

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// A cached ozone file that does not parse is replaced in the call that finds
// it. The endpoint is immutable, so GetFile reuses its copy on existence
// alone, and with the parser passed as WithValidate, which sees only a
// download, such a copy failed every call and was never replaced (#1043).
func TestOzoneReplacesADamagedCachedCopy(t *testing.T) {
	t.Cleanup(remote.Capture(remote.MPIMainzCrossSections).Restore)

	ctx := context.Background()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	srcURL := testutil.FileURL(t, t.TempDir())
	if err := remote.SetURL(remote.MPIMainzCrossSections, srcURL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	src, err := remote.OpenFS(ctx, srcURL)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}

	cache, prefix, err := remote.CacheDir(ctx, remote.MPIMainzCrossSections)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	remote.EnableDownloads(0, remote.MPIMainzCrossSections)

	good := chappuisFile(4.6e-21)
	if err := parseAndCheck(good); err != nil {
		t.Fatalf("the source file is not plausible ozone: %v", err)
	}

	key := prefix + remote.OzoneSerdyuchenko223K

	if err := remote.WriteFile(ctx, src, remote.OzoneSerdyuchenko223K, strings.NewReader(good)); err != nil {
		t.Fatalf("write the source: %v", err)
	}

	if err := remote.WriteFile(ctx, cache, key, strings.NewReader("<html>damaged</html>")); err != nil {
		t.Fatalf("write the damaged copy: %v", err)
	}

	xs, err := Ozone(ctx)
	if err != nil {
		t.Fatalf("Ozone over a damaged cached copy: %v", err)
	}

	if len(xs.WavelengthNM) != 31 {
		t.Errorf("%d samples, want the source's 31", len(xs.WavelengthNM))
	}

	cached, err := fs.ReadFile(cache, key)
	if err != nil || string(cached) != good {
		t.Errorf("the cached copy is not the source's after the heal (err %v)", err)
	}
}
