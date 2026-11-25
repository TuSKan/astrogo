package jpl_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// serveKernels points NAIFSPK at a local source holding the given kernels
// under planets/, NAIFLSK at one holding naif0012, and the cache at an empty
// directory, so NewProvider reaches no host.
func serveKernels(t *testing.T, kernels map[string][]byte) {
	t.Helper()

	t.Cleanup(remote.Capture(remote.NAIFSPK, remote.NAIFLSK).Restore)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	files := map[string][]byte{"lsk/naif0012.tls": []byte(testutil.NAIF0012)}
	for name, b := range kernels {
		files["planets/"+name+".bsp"] = b
	}

	src := testutil.FileURL(t, t.TempDir())

	fsys, err := remote.OpenFS(context.Background(), src)
	if err != nil {
		t.Fatalf("open the source: %v", err)
	}

	for key, b := range files {
		if err := remote.WriteFile(context.Background(), fsys, key, bytes.NewReader(b)); err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}

	for _, id := range []remote.EndpointID{remote.NAIFSPK, remote.NAIFLSK} {
		if err := remote.SetURL(id, src); err != nil {
			t.Fatal(err)
		}
	}
}

// TestNewProviderNamesTheExtraKernelStepThatFailed: a kernel added with
// WithKernel fails at one of two steps, fetching it or adding it, and the
// error says which and names the kernel.
func TestNewProviderNamesTheExtraKernelStepThatFailed(t *testing.T) {
	serveKernels(t, map[string][]byte{
		"base":     kernelInFrame(1),
		"ecliptic": kernelInFrame(17),
	})

	for _, c := range []struct {
		extra, step string
		want        error
	}{
		{"absent", "jpl: cache kernel absent", fs.ErrNotExist},
		{"ecliptic", "jpl: add kernel ecliptic", jpl.ErrUnsupportedFrame},
	} {
		_, err := jpl.NewProvider(context.Background(), core.Planets, "base", jpl.WithKernel(c.extra))
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.extra, err, c.want)

			continue
		}

		if !strings.Contains(err.Error(), c.step) {
			t.Errorf("%s: the error does not say %q:\n  %v", c.extra, c.step, err)
		}
	}
}

// TestNewProviderLoadsEachExtraKernel is the control for the test above: the
// same source, with kernels that can be fetched and added, each recorded
// under the key it was fetched as.
func TestNewProviderLoadsEachExtraKernel(t *testing.T) {
	serveKernels(t, map[string][]byte{
		"base":   kernelInFrame(1),
		"part-1": kernelInFrame(1),
		"part-2": kernelInFrame(1),
	})

	p, err := jpl.NewProvider(context.Background(), core.Planets, "base",
		jpl.WithKernel("part-1"), jpl.WithKernel("part-2"))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	t.Cleanup(func() { _ = p.Close() })

	loaded := p.LoadedKernels()

	keys := make([]string, 0, len(loaded))
	for _, k := range loaded {
		keys = append(keys, k.Key)
	}

	want := []string{"planets/base.bsp", "planets/part-1.bsp", "planets/part-2.bsp"}
	if !slices.Equal(keys, want) {
		t.Errorf("loaded kernels %q, want %q", keys, want)
	}
}
