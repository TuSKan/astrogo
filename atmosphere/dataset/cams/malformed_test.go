package cams

import (
	"context"
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/scigolib/hdf5"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// A malformed file is an error, never the end of the caller's process (#794).
//
// Each case is one byte of synthFixture's 426,636-byte file changed, found by
// overwriting random bytes of its metadata and reading every variant through
// Open, Var and ReadPlane. Before the fix the first three were a fatal "out of
// memory" — the decoder sized a buffer from what the file declared, 8.8 TB in
// the first — which no recover can catch, and the fourth a panic inside the
// decoder's own Open.

// fixtureSize pins the fixture the offsets below refer to. A change to
// synthFixture moves them, and has to be met by finding them again.
const fixtureSize = 426636

func corruptFixture(t *testing.T, offset int, value byte) (remote.FS, string) {
	t.Helper()

	fsys, key := synthFixture(t)

	r, err := remote.Open(context.Background(), fsys, key)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}

	b, err := io.ReadAll(r)
	_ = r.Close()

	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	if len(b) != fixtureSize {
		t.Fatalf("the fixture is %d bytes, not %d: synthFixture changed, and these offsets have to be found again", len(b), fixtureSize)
	}

	b[offset] = value

	out, err := remote.OpenFS(context.Background(), testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("open bucket: %v", err)
	}

	if err := remote.WriteFile(context.Background(), out, "corrupt.nc", strings.NewReader(string(b))); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	return out, "corrupt.nc"
}

func TestOpenRefusesAMalformedFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		offset int
		value  byte
		want   string
	}{
		{"a dimension scale claiming 2.2 trillion values", 5368, 0x02, "longitude declares 2199023255556 values"},
		{"a string attribute claiming 386 MB in 16 bytes", 5824, 0x17, `attribute "CLASS" declares`},
		{"an integer attribute claiming 16 GB in 4 bytes", 5953, 0xf2, `attribute "_Netcdf4Dimid" declares`},
		// Until scigolib/hdf5 v0.14.2 this byte led the decoder into a panic,
		// which decode recovers as "the decoder failed"; v0.14.2 refuses the
		// 720 PB local heap it makes on its own. Either way the file is
		// malformed, and the panic path is TestDecodeTurnsAPanicIntoAnError's.
		{"a local heap claiming 720 PB, refused by the decoder", 63, 0x0a, "exceeds maximum"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fsys, key := corruptFixture(t, tt.offset, tt.value)

			cf, err := Open(context.Background(), fsys, key)
			if err == nil {
				_ = cf.Close()

				t.Fatal("Open succeeded on a malformed file")
			}

			if !errors.Is(err, ErrMalformed) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Open = %v, want ErrMalformed saying %q", err, tt.want)
			}
		})
	}
}

// TestReadPlaneRefusesAnElementSizeTheFileCannotHold: the decoder sizes a
// plane's buffer as its values times the datatype's size per value, and
// v0.14.2 does not bound that product for the contiguous slice ReadPlane
// reads. CI found it: a corrupted element size asked for 40.9 GB to hold
// aermr01's 12 values, and ended the test binary on Linux and on Windows.
//
// Each offset is the third byte of the size in a float64 variable's datatype
// message, 08 00 00 00, so 0x10 there declares 1 MiB and 8 bytes a value.
func TestReadPlaneRefusesAnElementSizeTheFileCannotHold(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		offset int
	}{
		{"aermr01", 286194},
		{"lnsp", 356441},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fsys, key := corruptFixture(t, tt.offset, 0x10)

			cf, err := Open(context.Background(), fsys, key)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}

			defer func() { _ = cf.Close() }()

			v, err := cf.Var(tt.name)
			if err != nil {
				t.Fatalf("Var: %v", err)
			}

			_, err = v.ReadPlane(0, 0)

			want := tt.name + " declares 12 values of 1048584 bytes"
			if !errors.Is(err, ErrMalformed) || !strings.Contains(err.Error(), want) {
				t.Errorf("ReadPlane = %v, want ErrMalformed saying %q", err, want)
			}
		})
	}
}

// errDecoderRefused stands for an error the decoder returns about the file.
var errDecoderRefused = errors.New("local heap data: size exceeds maximum")

func TestDecodeTurnsAPanicIntoAnError(t *testing.T) {
	t.Parallel()

	err := decode("file.nc", "walk", func() error {
		panic("index out of range") // as a malformed structure leads a decoder into
	})

	if !errors.Is(err, ErrMalformed) || !strings.Contains(err.Error(), "file.nc: walk: the decoder failed") {
		t.Errorf("decode = %v, want ErrMalformed naming the file and the operation", err)
	}

	if err := decode("file.nc", "walk", func() error { return nil }); err != nil {
		t.Errorf("decode of a call that succeeds = %v, want nil", err)
	}

	// An error the decoder returns about the file is the file being
	// malformed, as a panic is; closing it is not about its content.
	refused := errDecoderRefused

	if err := decode("file.nc", "open", func() error { return refused }); !errors.Is(err, ErrMalformed) || !errors.Is(err, refused) {
		t.Errorf("decode of a call the decoder refuses = %v, want ErrMalformed wrapping the decoder's error", err)
	}

	if err := release("file.nc", func() error { return refused }); errors.Is(err, ErrMalformed) || !errors.Is(err, refused) {
		t.Errorf("release of a failing close = %v, want the close error, not ErrMalformed", err)
	}
}

// The bound reads the decoder's description of a dataset, its own string
// format, so the format is pinned here against the version in go.mod: every
// dataset in the fixture must be readable, and a description that is not
// must fail closed.
func TestCheckExtentReadsTheDecodersDescription(t *testing.T) {
	t.Parallel()

	fsys, key := synthFixture(t)

	cf, err := Open(context.Background(), fsys, key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = cf.Close() }()

	seen := 0

	cf.hf.Walk(func(_ string, obj hdf5.Object) {
		ds, ok := obj.(*hdf5.Dataset)
		if !ok {
			return
		}

		info, err := ds.Info()
		if err != nil {
			t.Errorf("%s: Info: %v", ds.Name(), err)

			return
		}

		if _, ok := declaredValues(info); !ok {
			t.Errorf("%s: the bound cannot read %q: the decoder's format changed", ds.Name(), info)
		}

		if _, ok := elementSize(info); !ok {
			t.Errorf("%s: the bound cannot read the element size in %q: the decoder's format changed", ds.Name(), info)
		}

		seen++
	})

	if seen == 0 {
		t.Fatal("the fixture had no datasets to check")
	}

	for _, tt := range []struct {
		info string
		want uint64
		ok   bool
	}{
		{"Dataset: float32, 1D array [4], contiguous (address=0x800, size=16)", 4, true},
		{"Dataset: float64, 2D array [3 x 4], contiguous (address=0x800, size=96)", 12, true},
		{"Dataset: float64, 4D array [2 3 3 4], chunked (chunks=[1 1 3 4])", 72, true},
		{"Dataset: int32, scalar, compact (size=4)", 1, true},
		{"Dataset: int32, null, compact (size=0)", 0, true},
		{"Dataset: float64, 2D array [18446744073709551615 x 2], contiguous (address=0x0, size=0)", math.MaxUint64, true},
		{"Dataset: float64, a shape this reader has never seen", 0, false},
	} {
		got, ok := declaredValues(tt.info)
		if got != tt.want || ok != tt.ok {
			t.Errorf("declaredValues(%q) = %d, %v; want %d, %v", tt.info, got, ok, tt.want, tt.ok)
		}
	}

	// The datatype's size is "bytes"; the layout's, in a compact or contiguous
	// description, is not, and must not be read as the element size.
	for _, tt := range []struct {
		info string
		want uint64
		ok   bool
	}{
		{"Dataset: float (size=8 bytes), 4D array [1 2 3 4], contiguous (address=0x45D21, size=192)", 8, true},
		{"Dataset: integer (size=4 bytes), 1D array [2], contiguous (address=0x238C7, size=8)", 4, true},
		{"Dataset: float (size=4294967295 bytes), 1D array [3], chunked (chunks=[3 4])", math.MaxUint32, true},
		{"Dataset: integer, scalar, compact (size=4)", 0, false},
	} {
		got, ok := elementSize(tt.info)
		if got != tt.want || ok != tt.ok {
			t.Errorf("elementSize(%q) = %d, %v; want %d, %v", tt.info, got, ok, tt.want, tt.ok)
		}
	}
}
