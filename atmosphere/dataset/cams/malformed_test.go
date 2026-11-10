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
// Each case is one byte of synthFixture's file changed. They were first found
// by overwriting random bytes of its metadata and reading every variant
// through Open, Var and ReadPlane. Before the fix the first three were a fatal
// "out of memory" (the decoder sized a buffer from what the file declared,
// 8.8 TB in the first), which no recover can catch, and the fourth a panic
// inside the decoder's own Open.
//
// Each offset is a field of the HDF5 format, named in its case, so a new
// writer layout is met by finding the same fields again rather than by
// searching. scigolib/hdf5 v0.15.0 moved every one of them: it writes a
// dataset's attributes as compact messages in its object header, where v0.14.2
// kept them in dense storage, and the file shrank from 426,636 bytes to 8,090
// (#1024). The v0.15.0 object headers, from the start of the file:
// longitude's at 5320, latitude's, level's, time's, then aermr01's at 7099
// and lnsp's at 7691.

// fixtureSize pins the fixture the offsets below refer to. A change to
// synthFixture moves them, and has to be met by finding them again.
const fixtureSize = 8090

// corruptFixture writes synthFixture with each byte at an offset in edits
// replaced by its value.
func corruptFixture(t *testing.T, edits map[int]byte) (remote.FS, string) {
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

	for offset, value := range edits {
		b[offset] = value
	}

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
		name  string
		edits map[int]byte
		want  string
	}{
		// Byte 5 of the first dimension, 4, in longitude's dataspace message:
		// the first message of its object header, a version 2 dataspace
		// whose dimensions start 4 bytes in, at 5336.
		//
		// v0.15.0 refuses it on its own: it checks a dimension against the
		// maximum the dataspace also stores, 4 here. Raising that maximum to
		// match does not reach this package's bound either, since v0.15.0
		// then bounds the contiguous storage the dataset declares, by the
		// file and by 1 TiB, before checkExtent can read it. For a contiguous
		// dataset that bound is now the decoder's, and checkExtent the second
		// line behind it.
		{"a dimension scale claiming 2.2 trillion values, refused by the decoder",
			map[int]byte{5341: 0x02}, "dimension 0 size 2199023255556 is greater than its maximum 4"},
		// Byte 3 of the size, 16, in the string datatype of longitude's CLASS
		// attribute: the attribute message's name, "CLASS\0", ends at 5423,
		// and the datatype's size follows its 4-byte header.
		{"a string attribute claiming 386 MB in 16 bytes",
			map[int]byte{5430: 0x17}, `attribute "CLASS" declares`},
		// Byte 3 of the one dimension, 1, in the dataspace of longitude's
		// _Netcdf4Dimid attribute: a version 2 dataspace after the
		// attribute's 12-byte integer datatype, its dimension at 5566 and
		// its maximum, also 1, at 5574. Both are raised, since v0.15.0
		// refuses a dimension past its maximum before this package's
		// attribute bound sees it.
		{"an integer attribute claiming 16 GB in 4 bytes",
			map[int]byte{5569: 0xf2, 5577: 0xf2}, `attribute "_Netcdf4Dimid" declares`},
		// Byte 7 of the data segment size in the root group's local heap,
		// whose "HEAP" signature is at 48 in both writers' files.
		//
		// Until scigolib/hdf5 v0.14.2 this byte led the decoder into a panic,
		// which decode recovers as "the decoder failed"; v0.14.2 and later
		// refuse the 720 PB local heap it makes on their own. Either way the
		// file is malformed, and the panic path is
		// TestDecodeTurnsAPanicIntoAnError's.
		{"a local heap claiming 720 PB, refused by the decoder",
			map[int]byte{63: 0x0a}, "exceeds maximum"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fsys, key := corruptFixture(t, tt.edits)

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
// message, 08 00 00 00, so 0x10 there declares 1 MiB and 8 bytes a value. The
// datatype message is the second in each variable's object header, after its
// dataspace: at 7183 for aermr01 and 7759 for lnsp, its size 4 bytes in.
//
// v0.15.0 now bounds a contiguous dataset's storage by the file itself, in
// the Info that checkExtent reads before computing its own bound, and that
// check is the tighter of the two: it counts the storage's offset in the
// file. So the refusal is the decoder's, arriving as ErrMalformed through
// checkExtent, and this package's bound is the second line behind it.
func TestReadPlaneRefusesAnElementSizeTheFileCannotHold(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		offset int
	}{
		{"aermr01", 7189},
		{"lnsp", 7765},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fsys, key := corruptFixture(t, map[int]byte{tt.offset: 0x10})

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

			want := `dataset "` + tt.name + `": hdf5: corrupt file: contiguous dataset storage: read beyond end of file`
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
