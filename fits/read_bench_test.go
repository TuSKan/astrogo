package fits_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"strconv"
	"testing"

	"github.com/klauspost/pgzip"

	"github.com/TuSKan/astrogo/fits"
)

// benchSources reads one file the two ways Read is handed one: a seekable
// source, as fits.Open passes a plain file, and a stream that cannot say what
// it holds, as it passes a .fits.gz.
var benchSources = []struct {
	name string
	open func([]byte) io.Reader
}{
	{"seekable", func(b []byte) io.Reader { return bytes.NewReader(b) }},
	{"stream", func(b []byte) io.Reader { return onlyReader{bytes.NewReader(b)} }},
}

func benchmarkRead(b *testing.B, raw []byte) {
	b.Helper()

	for _, source := range benchSources {
		b.Run(source.name, func(b *testing.B) {
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()

			for b.Loop() {
				if _, err := fits.Read(source.open(raw)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}

	// The stream fits.Open actually builds: a .fits.gz through pgzip, where
	// decompression rather than the read buffer sets the pace.
	var gz bytes.Buffer

	w := pgzip.NewWriter(&gz)
	if _, err := w.Write(raw); err != nil {
		b.Fatal(err)
	}

	if err := w.Close(); err != nil {
		b.Fatal(err)
	}

	b.Run("gzip", func(b *testing.B) {
		b.SetBytes(int64(len(raw)))
		b.ReportAllocs()

		for b.Loop() {
			zr, err := pgzip.NewReader(bytes.NewReader(gz.Bytes()))
			if err != nil {
				b.Fatal(err)
			}

			if _, err := fits.Read(zr); err != nil {
				b.Fatal(err)
			}

			if err := zr.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkReadImage reads a 2048 × 2048 float32 image, 16 MiB of pixels.
func BenchmarkReadImage(b *testing.B) {
	benchmarkRead(b, imageFITS(2048, 2048, func(x, y int) float32 { return float32(x ^ y) }))
}

// BenchmarkReadBintable reads a one-column table of a million float64 rows,
// 8 MB of data.
func BenchmarkReadBintable(b *testing.B) {
	const rows = 1_000_000

	data := make([]byte, 8*rows)
	for i := range rows {
		binary.BigEndian.PutUint64(data[8*i:], math.Float64bits(float64(i)/3))
	}

	raw := append(emptyPrimary(), hduBytes(data,
		card("XTENSION", "'BINTABLE'"), card("BITPIX", "8"), card("NAXIS", "2"),
		card("NAXIS1", "8"), card("NAXIS2", strconv.Itoa(rows)), card("PCOUNT", "0"), card("GCOUNT", "1"),
		card("TFIELDS", "1"), card("TTYPE1", "'FLUX    '"), card("TFORM1", "'D       '"))...)

	benchmarkRead(b, raw)
}
