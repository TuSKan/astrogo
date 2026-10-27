package cams

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"runtime"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// fixtureBytes is synthFixture's file, as bytes.
func fixtureBytes(tb testing.TB) []byte {
	tb.Helper()

	fsys, key := synthFixture(tb)

	r, err := remote.Open(context.Background(), fsys, key)
	if err != nil {
		tb.Fatalf("open fixture: %v", err)
	}

	b, err := io.ReadAll(r)
	_ = r.Close()

	if err != nil {
		tb.Fatalf("read fixture: %v", err)
	}

	return b
}

// camsErrors are the errors this package reports a file with. A malformed file
// may produce any of them; what it may not produce is anything else, such as
// a decoder's own error passed through, which would leave a caller unable to
// tell a bad file from a broken reader.
var camsErrors = []error{
	ErrMalformed, ErrVariableNotFound, ErrUnsupportedAxis, ErrUnexpectedAttributeType,
	ErrNoLevelDimension, ErrIndexOutOfRange,
}

// isCAMSError reports whether err is one of camsErrors.
func isCAMSError(err error) bool {
	for _, want := range camsErrors {
		if errors.Is(err, want) {
			return true
		}
	}

	return false
}

// readEverything opens data as a CAMS file and reads every plane of every
// variable the fixture holds, from a bucket it writes it to, and returns how
// many planes it read. Every error has to be one of this package's own; what
// is guarded beyond that is that nothing panics and nothing ends the process,
// which no recover could catch (#794).
func readEverything(tb testing.TB, fsys remote.FS, data []byte) int {
	tb.Helper()

	ctx := context.Background()

	if err := remote.WriteFile(ctx, fsys, "fuzz.nc", bytes.NewReader(data)); err != nil {
		tb.Fatalf("write: %v", err)
	}

	cf, err := Open(ctx, fsys, "fuzz.nc")
	if err != nil {
		if !isCAMSError(err) {
			tb.Errorf("Open = %v, not one of this package's errors", err)
		}

		return 0
	}

	defer func() { _ = cf.Close() }()

	planes := 0

	for _, name := range []string{"aermr01", "lnsp", "longitude", "latitude", "level", "time"} {
		v, err := cf.Var(name)
		if err != nil {
			if !isCAMSError(err) {
				tb.Errorf("Var(%q) = %v, not one of this package's errors", name, err)
			}

			continue
		}

		levels := 1
		if v.HasLevel() {
			levels = min(max(cf.Dims()["level"], 1), 4)
		}

		for level := range levels {
			if _, err := v.ReadPlane(0, level); err != nil {
				if !isCAMSError(err) {
					tb.Errorf("%s.ReadPlane(0, %d) = %v, not one of this package's errors", name, level, err)
				}

				continue
			}

			planes++
		}
	}

	return planes
}

// FuzzOpen feeds Open arbitrary bytes and reads everything it opens.
//
// The seeds are the synthetic fixture and the four single-byte corruptions of
// it TestOpenRefusesAMalformedFile names, three of which once made the decoder
// allocate terabytes. #795 held this target back: the decoder's own local
// heap, allocated from its declared size, was a path it could not close, and
// this table's claim is never-panics. scigolib/hdf5 v0.14.2 bounds that
// allocation, and checkExtent the contiguous slice it does not.
func FuzzOpen(f *testing.F) {
	fixture := fixtureBytes(f)
	f.Add(fixture)

	for _, c := range []struct {
		offset int
		value  byte
	}{{5368, 0x02}, {5824, 0x17}, {5953, 0xf2}, {63, 0x0a}} {
		b := bytes.Clone(fixture)
		b[c.offset] = c.value
		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		fsys, err := remote.OpenFS(context.Background(), testutil.FileURL(t, t.TempDir()))
		if err != nil {
			t.Fatalf("open bucket: %v", err)
		}

		readEverything(t, fsys, data)
	})
}

// corruptSweep is how many corrupted variants TestOpenSurvivesCorruptions
// reads, and how many under -short. Against scigolib/hdf5 v0.14.1 this seed's
// variant 123 ends the process, so both reach the failure the test exists for;
// all 6,000 #795 ran pass against v0.14.2, in about a minute.
const (
	corruptSweep      = 1000
	corruptSweepShort = 300
)

// TestOpenSurvivesCorruptions overwrites one to four of the fixture's bytes
// with random values, in corruptSweep variants from a fixed seed, and opens
// and reads each in full. None may end the process, or allocate more than a
// file of the fixture's size could need.
//
// The bytes are drawn from the file's structure: the runs of its non-zero
// bytes, zeros inside them included, 6,097 of its 426,636 bytes between long
// stretches of zero padding. The zeros matter. Drawn from the non-zero bytes
// alone, the sweep never touched the high bytes of a size field, which are
// what declares petabytes, and passed against v0.14.1 as well.
//
// #795 ran such a sweep. Before it, the run died at the first allocation it
// could not make; after it, 2 variants in 6,000 still did, inside the
// decoder's own LoadLocalHeap, which only the decoder could bound. Against
// v0.14.1 this one dies there too, at variant 123, asking for 213 TB. With
// scigolib/hdf5 v0.14.2 it completes, which is the measurement: a fatal
// out-of-memory is not an error this test could report, it is the end of the
// test binary.
//
// That much measures the machine as well as the reader. With v0.14.2 the
// sweep passed here while variants 334 and 744 asked for 40.9 and 50.7 GB, for
// contiguous planes with a corrupted element size, which this machine could
// commit and Linux and Windows CI could not. -short stops before both, and
// TestReadPlaneRefusesAnElementSizeTheFileCannotHold holds that case in every
// mode. So each variant's allocations are held to
// what the fixture's bytes could hold at DEFLATE's largest expansion, the
// bound checkExtent applies to compressed data: anything more was sized from
// a declaration no file of this size could keep. TotalAlloc is the process's,
// so a parallel test's allocations count too; they are megabytes, against
// 440 MB, and the largest variant allocates 3 MB.
func TestOpenSurvivesCorruptions(t *testing.T) {
	t.Parallel()

	fixture := fixtureBytes(t)

	structure := structureOffsets(fixture)

	n := corruptSweep
	if testing.Short() {
		n = corruptSweepShort
	}

	fsys, err := remote.OpenFS(context.Background(), testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("open bucket: %v", err)
	}

	rng := rand.New(rand.NewPCG(794, 795))
	read := 0
	limit := uint64(deflateMaxRatio * len(fixture))

	for i := range n {
		b := bytes.Clone(fixture)

		for range 1 + rng.IntN(4) {
			b[structure[rng.IntN(len(structure))]] = byte(rng.IntN(256))
		}

		var before, after runtime.MemStats

		runtime.ReadMemStats(&before)

		if readEverything(t, fsys, b) > 0 {
			read++
		}

		runtime.ReadMemStats(&after)

		if allocated := after.TotalAlloc - before.TotalAlloc; allocated > limit {
			t.Errorf("variant %d allocated %d bytes reading a %d-byte file, more than one could hold, %d",
				i, allocated, len(fixture), limit)
		}
	}

	// A reader that refused every variant would pass this vacuously.
	if read == 0 {
		t.Fatalf("none of %d variants read a plane: the sweep exercised nothing past Open", n)
	}

	t.Logf("%d of %d corrupted variants still read at least one plane", read, n)
}

// structureOffsets lists the offsets of data's structure: every byte of each
// run of non-zero bytes, where a run ends only at 64 zeros in a row.
func structureOffsets(data []byte) []int {
	const gap = 64

	var out []int

	for i := 0; i < len(data); {
		if data[i] == 0 {
			i++

			continue
		}

		end, zeros := i, 0
		for j := i; j < len(data) && zeros < gap; j++ {
			if data[j] == 0 {
				zeros++
			} else {
				end, zeros = j+1, 0
			}
		}

		for k := i; k < end; k++ {
			out = append(out, k)
		}

		i = end
	}

	return out
}
