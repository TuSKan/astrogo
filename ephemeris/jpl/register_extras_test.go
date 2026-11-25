package jpl_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// naif0012 is the data of NAIF's naif0012.tls, the leap-second kernel
// NewProvider loads, copied from the kernel. Its DELTA_AT table has to be the
// whole one: NewProvider registers it, and a table that disagrees with the
// built-in record is refused.
const naif0012 = `KPL/LSK

\begindata

DELTET/DELTA_T_A       =   32.184
DELTET/K               =    1.657D-3
DELTET/EB              =    1.671D-2
DELTET/M               = (  6.239996D0   1.99096871D-7 )

DELTET/DELTA_AT        = ( 10,   @1972-JAN-1
                           11,   @1972-JUL-1
                           12,   @1973-JAN-1
                           13,   @1974-JAN-1
                           14,   @1975-JAN-1
                           15,   @1976-JAN-1
                           16,   @1977-JAN-1
                           17,   @1978-JAN-1
                           18,   @1979-JAN-1
                           19,   @1980-JAN-1
                           20,   @1981-JUL-1
                           21,   @1982-JUL-1
                           22,   @1983-JUL-1
                           23,   @1985-JUL-1
                           24,   @1988-JAN-1
                           25,   @1990-JAN-1
                           26,   @1991-JAN-1
                           27,   @1992-JUL-1
                           28,   @1993-JUL-1
                           29,   @1994-JUL-1
                           30,   @1996-JAN-1
                           31,   @1997-JUL-1
                           32,   @1999-JAN-1
                           33,   @2006-JAN-1
                           34,   @2009-JAN-1
                           35,   @2012-JUL-1
                           36,   @2015-JUL-1
                           37,   @2017-JAN-1 )

\begintext
`

// serveKernels points NAIFSPK at a local source holding the given kernels
// under planets/, NAIFLSK at one holding naif0012, and the cache at an empty
// directory, so NewProvider reaches no host.
func serveKernels(t *testing.T, kernels map[string][]byte) {
	t.Helper()

	t.Cleanup(remote.Capture(remote.NAIFSPK, remote.NAIFLSK).Restore)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	files := map[string][]byte{"lsk/naif0012.tls": []byte(naif0012)}
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

// TestBackendNamesTheExtraKernelStepThatFailed: a kernel added with
// ephemeris.WithKernel fails at one of two steps, fetching it or adding it,
// and the error says which, naming the ephemeris package once.
func TestBackendNamesTheExtraKernelStepThatFailed(t *testing.T) {
	serveKernels(t, map[string][]byte{
		"base":     kernelInFrame(1),
		"ecliptic": kernelInFrame(17),
	})

	for _, c := range []struct {
		extra, step string
		want        error
	}{
		{"absent", "jpl: cache kernel absent", fs.ErrNotExist},
		{"ecliptic", "jpl: add kernel", jpl.ErrUnsupportedFrame},
	} {
		_, err := ephemeris.NewProvider(context.Background(), ephemeris.Planets, "base", ephemeris.WithKernel(c.extra))
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.extra, err, c.want)

			continue
		}

		if !strings.Contains(err.Error(), c.step) {
			t.Errorf("%s: the error does not say %q:\n  %v", c.extra, c.step, err)
		}

		if got := strings.Count(err.Error(), "ephemeris:"); got != 1 {
			t.Errorf("%s: the error names the ephemeris package %d times, want once:\n  %v", c.extra, got, err)
		}
	}
}

// TestBackendLoadsAnExtraKernel is the control for the test above: the same
// source, with an extra kernel that can be fetched and added.
func TestBackendLoadsAnExtraKernel(t *testing.T) {
	serveKernels(t, map[string][]byte{
		"base":  kernelInFrame(1),
		"extra": kernelInFrame(1),
	})

	p, err := ephemeris.NewProvider(context.Background(), ephemeris.Planets, "base", ephemeris.WithKernel("extra"))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	jp, ok := p.(*jpl.Provider)
	if !ok {
		t.Fatalf("NewProvider returned a %T, want *jpl.Provider", p)
	}

	if n := len(jp.LoadedKernels()); n != 2 {
		t.Errorf("%d kernels loaded, want the base and the extra", n)
	}
}
