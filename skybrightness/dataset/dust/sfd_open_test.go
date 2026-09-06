package dust

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/file"
)

// The real hemispheres are 64 MB each and live behind a Dataverse download, so
// nothing offline can exercise Open against them. What can be exercised is
// every reason Open refuses a file — and those are the failure modes that
// actually happen: a deposit reorganised, an identifier repointed at the
// reddening map, a header keyword dropped.
//
// The fixtures below are 2x2 SFD-shaped images built here rather than checked
// in, so what each test is varying is visible in the test itself.

const (
	fitsBlock = 2880
	fitsCard  = 80
)

// sfdHeader is the keyword set a hemisphere must carry. Values are strings
// exactly as they appear on the card, so a test can corrupt one by name.
func sfdHeader(nsgp string) map[string]string {
	return map[string]string{
		"BUNIT":    "'MJy/sr '",
		"CTYPE1":   "'GLON-ZEA'",
		"LAM_NSGP": nsgp,
		"LAM_SCAL": "2",
		"CRPIX1":   "1.5",
		"CRPIX2":   "1.5",
	}
}

// buildFITS renders a single-HDU image at the given BITPIX. Order matters —
// SIMPLE, BITPIX, NAXIS and the NAXISn cards are mandatory and positional — so
// they are written first and the rest follow in the order given.
//
// bitpix decides how each value is encoded: -32 and -64 are the two floating
// forms SFD uses and pixels accepts, 16 the integer form it refuses.
func buildFITS(t *testing.T, bitpix int, naxis []int, keys map[string]string, order []string, data []float64) []byte {
	t.Helper()

	var head bytes.Buffer

	card := func(k, v string) {
		t.Helper()

		line := fmt.Sprintf("%-8s= %20s", k, v)
		head.WriteString(line + strings.Repeat(" ", fitsCard-len(line)))
	}

	card("SIMPLE", "T")
	card("BITPIX", strconv.Itoa(bitpix))
	card("NAXIS", strconv.Itoa(len(naxis)))

	for i, n := range naxis {
		card(fmt.Sprintf("NAXIS%d", i+1), strconv.Itoa(n))
	}

	for _, k := range order {
		if v, ok := keys[k]; ok {
			card(k, v)
		}
	}

	head.WriteString("END" + strings.Repeat(" ", fitsCard-3))

	for head.Len()%fitsBlock != 0 {
		head.WriteString(strings.Repeat(" ", fitsCard))
	}

	var payload bytes.Buffer

	for _, v := range data {
		switch bitpix {
		case -32:
			_ = binary.Write(&payload, binary.BigEndian, math.Float32bits(float32(v)))
		case -64:
			_ = binary.Write(&payload, binary.BigEndian, math.Float64bits(v))
		case 16:
			_ = binary.Write(&payload, binary.BigEndian, int16(v))
		default:
			t.Fatalf("buildFITS: BITPIX %d not supported by this helper", bitpix)
		}
	}

	out := append(head.Bytes(), payload.Bytes()...)
	for len(out)%fitsBlock != 0 {
		out = append(out, 0)
	}

	return out
}

// keyOrder is the order the optional cards are written in; every fixture uses
// it so a test varies only the values.
var keyOrder = []string{"BUNIT", "CTYPE1", "LAM_NSGP", "LAM_SCAL", "CRPIX1", "CRPIX2"}

// hemisphereFITS is a well-formed 2x2 float32 hemisphere with the four given
// pixels.
func hemisphereFITS(t *testing.T, nsgp string, px [4]float64) []byte {
	t.Helper()

	return buildFITS(t, -32, []int{2, 2}, sfdHeader(nsgp), keyOrder, px[:])
}

// serveSFD points remote.SFDDustMap at a bucket holding the two hemisphere
// files, and grants the download consent Open needs.
func serveSFD(t *testing.T, north, south []byte) {
	t.Helper()

	t.Cleanup(remote.Capture().Restore)

	ctx := context.Background()
	src := testutil.FileURL(t, t.TempDir())

	if err := remote.SetURL(remote.SFDDustMap, src); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	bucket, err := file.Open(ctx, src)
	if err != nil {
		t.Fatalf("open source bucket: %v", err)
	}

	for name, body := range map[string][]byte{northFile: north, southFile: south} {
		if err := bucket.WriteAll(ctx, name, body, nil); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	remote.EnableDownloads(0, remote.SFDDustMap)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
}

// TestSFDOpenReadsBothHemispheres is the happy path Open never had:
// two well-formed files, decoded, checked against each other and queried.
func TestSFDOpenReadsBothHemispheres(t *testing.T) {
	serveSFD(t,
		hemisphereFITS(t, "1", [4]float64{1, 2, 3, 4}),
		hemisphereFITS(t, "-1", [4]float64{5, 6, 7, 8}),
	)

	sfd, err := Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// The north file is the one with LAM_NSGP +1, whichever order they were
	// read in.
	if sfd.north.nsgp != 1 || sfd.south.nsgp != -1 {
		t.Errorf("hemispheres came back as nsgp %g and %g, want +1 and -1",
			sfd.north.nsgp, sfd.south.nsgp)
	}

	if sfd.north.width != 2 || sfd.north.height != 2 {
		t.Errorf("north is %dx%d, want 2x2", sfd.north.width, sfd.north.height)
	}

	if len(sfd.north.values) != 4 {
		t.Fatalf("north holds %d pixels, want 4", len(sfd.north.values))
	}

	for i, want := range []float64{1, 2, 3, 4} {
		if math.Abs(sfd.north.values[i]-want) > 1e-6 {
			t.Errorf("north pixel %d = %v, want %v", i, sfd.north.values[i], want)
		}
	}
}

// TestSFDOpenRefusesTwoOfTheSameHemisphere: the identifiers are Dataverse file
// numbers, not names, so a deposit reorganised under them is not visibly
// wrong. LAM_NSGP is what says which hemisphere a file actually is, and two
// norths would otherwise leave half the sky answered from the wrong map.
func TestSFDOpenRefusesTwoOfTheSameHemisphere(t *testing.T) {
	north := hemisphereFITS(t, "1", [4]float64{1, 2, 3, 4})

	serveSFD(t, north, north)

	_, err := Open(context.Background())
	if !errors.Is(err, ErrSFD) {
		t.Fatalf("err = %v, want ErrSFD", err)
	}

	if !strings.Contains(err.Error(), "opposite hemispheres") {
		t.Errorf("the error does not say what is wrong: %v", err)
	}
}

// TestSFDOpenRefusesAMisdescribedFile covers the header checks one at a time.
// Each is a way the deposit can change without the identifier changing, and
// each produces a map of plausible values that is not the one asked for.
func TestSFDOpenRefusesAMisdescribedFile(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]string)
		axes   []int
		want   string
	}{
		{
			name:   "reddening map instead of the 100 micron one",
			mutate: func(k map[string]string) { k["BUNIT"] = "'mag     '" },
			want:   "MJy/sr",
		},
		{
			name:   "a different projection",
			mutate: func(k map[string]string) { k["CTYPE1"] = "'GLON-TAN'" },
			want:   "equal-area",
		},
		{
			name:   "no scale keyword",
			mutate: func(k map[string]string) { delete(k, "LAM_SCAL") },
			want:   "usable projection",
		},
		{
			name:   "a scale of zero",
			mutate: func(k map[string]string) { k["LAM_SCAL"] = "0" },
			want:   "usable projection",
		},
		{
			name:   "no pole position",
			mutate: func(k map[string]string) { delete(k, "CRPIX1") },
			want:   "usable projection",
		},
		{
			name:   "not a two-dimensional image",
			mutate: func(map[string]string) {},
			axes:   []int{2, 2, 2},
			want:   "want 2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys := sfdHeader("1")
			tc.mutate(keys)

			axes := tc.axes
			if axes == nil {
				axes = []int{2, 2}
			}

			n := 1
			for _, a := range axes {
				n *= a
			}

			north := buildFITS(t, -32, axes, keys, keyOrder, make([]float64, n))

			serveSFD(t, north, hemisphereFITS(t, "-1", [4]float64{5, 6, 7, 8}))

			_, err := Open(context.Background())
			if !errors.Is(err, ErrSFD) {
				t.Fatalf("err = %v, want ErrSFD", err)
			}

			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the error does not mention %q: %v", tc.want, err)
			}
		})
	}
}

// TestSFDOpenReportsAMissingFile: without download consent nothing can be
// fetched, and Open has to say so rather than come back with an empty map that
// answers every direction with zero dust.
func TestSFDOpenReportsAMissingFile(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)

	if err := remote.SetURL(remote.SFDDustMap, testutil.FileURL(t, t.TempDir())); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	// Downloads deliberately not enabled.
	if _, err := Open(context.Background()); !errors.Is(err, ErrSFD) {
		t.Errorf("err = %v, want ErrSFD", err)
	}
}

// TestSFDOpenReadsAFloat64Image: the decoder types the payload from BITPIX, so
// -64 arrives as a different tensor than -32 and takes its own branch. SFD
// ships as -32, but the deposit is not this package's to control.
func TestSFDOpenReadsAFloat64Image(t *testing.T) {
	north := buildFITS(t, -64, []int{2, 2}, sfdHeader("1"), keyOrder, []float64{1, 2, 3, 4})

	serveSFD(t, north, hemisphereFITS(t, "-1", [4]float64{5, 6, 7, 8}))

	sfd, err := Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	for i, want := range []float64{1, 2, 3, 4} {
		if math.Abs(sfd.north.values[i]-want) > 1e-9 {
			t.Errorf("north pixel %d = %v, want %v", i, sfd.north.values[i], want)
		}
	}
}

// TestSFDOpenRefusesAnIntegerImage: an integer BITPIX would need BSCALE and
// BZERO applied per pixel, and the package says so rather than guessing. The
// refusal is the point — reading the raw integers as physical values would
// produce a map that is wrong by whatever scaling the file declares, silently.
func TestSFDOpenRefusesAnIntegerImage(t *testing.T) {
	north := buildFITS(t, 16, []int{2, 2}, sfdHeader("1"), keyOrder, []float64{1, 2, 3, 4})

	serveSFD(t, north, hemisphereFITS(t, "-1", [4]float64{5, 6, 7, 8}))

	_, err := Open(context.Background())
	if !errors.Is(err, ErrSFD) {
		t.Fatalf("err = %v, want ErrSFD", err)
	}

	if !strings.Contains(err.Error(), "floating") {
		t.Errorf("the error does not say why an integer image is refused: %v", err)
	}
}

// TestSFDOpenRefusesSomethingThatIsNotAnImage: the identifier addresses a
// deposited file, so what comes back need not be an image at all.
func TestSFDOpenRefusesSomethingThatIsNotAnImage(t *testing.T) {
	serveSFD(t,
		[]byte(strings.Repeat("not a FITS file ", fitsBlock/16)),
		hemisphereFITS(t, "-1", [4]float64{5, 6, 7, 8}),
	)

	if _, err := Open(context.Background()); !errors.Is(err, ErrSFD) {
		t.Errorf("err = %v, want ErrSFD", err)
	}
}
