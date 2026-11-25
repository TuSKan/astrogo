package solar_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/TuSKan/astrogo/internal/remotetest"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/solar"
)

// serveCALSPEC points remote.CALSPEC at a local bucket holding a solar
// reference that parses, and the cache at an empty one. Everything it changes
// is put back when the test ends.
//
// The source holding a valid file is what lets a refusal test fail: were the
// policy to let the download through, Open would return a spectrum rather
// than an error.
func serveCALSPEC(t *testing.T) context.Context {
	t.Helper()

	t.Cleanup(remote.Capture(remote.CALSPEC).Restore)

	ctx := context.Background()
	src := testutil.FileURL(t, t.TempDir())

	if err := remote.SetURL(remote.CALSPEC, src); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	bucket, err := remote.OpenFS(ctx, src)
	if err != nil {
		t.Fatalf("open source bucket: %v", err)
	}

	raw := calspecLike(t, []float64{4000, 5000, 6000}, []float64{2.5, 4.0, 1.0})
	if err := remote.WriteFile(ctx, bucket, solar.CALSPECSolarReference, bytes.NewReader(raw)); err != nil {
		t.Fatalf("seed %s: %v", solar.CALSPECSolarReference, err)
	}

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	return ctx
}

// TestOpenUnderARefusingPolicyIsRefused holds solar.Open to the download
// policy with nothing cached: the actionable ErrDownloadDenied, and nothing
// written to the cache. Until #967 the only test of this ran under the network
// tag, allowed every endpoint where it meant to refuse one, and passed
// whatever Open returned.
func TestOpenUnderARefusingPolicyIsRefused(t *testing.T) {
	ctx := serveCALSPEC(t)
	remotetest.RefuseDownloads(t, remote.CALSPEC)

	spectrum, err := solar.Open(ctx)
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("Open under a refusing policy = (%v, %v), want ErrDownloadDenied", spectrum, err)
	}

	cache, prefix, err := remote.CacheDir(ctx, remote.CALSPEC)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	if _, err := fs.Stat(cache, prefix+solar.CALSPECSolarReference); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the cache holds %s after a refused Open (stat: %v)", solar.CALSPECSolarReference, err)
	}
}

// TestOpenWithDownloadsAllowedReadsTheSource is the control for the test
// above: the same source, with downloads allowed, is downloaded, parsed and
// named. Without it a refusal could come from a fixture Open cannot read at
// all.
func TestOpenWithDownloadsAllowedReadsTheSource(t *testing.T) {
	ctx := serveCALSPEC(t)

	spectrum, err := solar.Open(ctx)
	if err != nil {
		t.Fatalf("Open with downloads allowed: %v", err)
	}

	if got := len(spectrum.WavelengthNM); got != 3 {
		t.Errorf("read %d rows, want the fixture's 3", got)
	}

	if spectrum.Source != solar.CALSPECSolarReference {
		t.Errorf("Source = %q, want %q", spectrum.Source, solar.CALSPECSolarReference)
	}
}
