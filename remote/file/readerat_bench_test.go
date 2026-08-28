package file

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// plainReaderAt is the "no storage at all" alternative: one NewRangeReader
// per ReadAt. Kept here only to measure it against ReaderAt.
type plainReaderAt struct {
	bucket *Bucket
	key    string
	calls  *atomic.Int64
}

func (p plainReaderAt) ReadAt(b []byte, off int64) (int, error) {
	p.calls.Add(1)

	rc, err := p.bucket.NewRangeReader(context.Background(), p.key, off, int64(len(b)), nil)
	if err != nil {
		return 0, fmt.Errorf("range read at %d: %w", off, err)
	}

	n, err := io.ReadFull(rc, b)
	_ = rc.Close()

	if err != nil {
		return n, fmt.Errorf("read at %d: %w", off, err)
	}

	return n, nil
}

// spkLikeOffsets is the access pattern SPK segment evaluation produces:
// many small reads, clustered inside a segment, jumping between segments.
func spkLikeOffsets(size int64, n int) []int64 {
	offs := make([]int64, 0, n)
	// Three hot regions, walked with small strides, revisited in turn.
	regions := []int64{0, size / 3, (size * 2) / 3}

	for i := range n {
		base := regions[i%len(regions)]
		off := base + int64((i/len(regions))%2048)*104

		if off+104 > size {
			off = base
		}

		offs = append(offs, off)
	}

	return offs
}

func benchPattern(b *testing.B, sizeMB int, read func(off int64) error, n int) {
	b.Helper()

	size := int64(sizeMB) << 20
	offs := spkLikeOffsets(size, n)

	b.ResetTimer()

	for range b.N {
		for _, off := range offs {
			if err := read(off); err != nil {
				b.Fatalf("read at %d: %v", off, err)
			}
		}
	}
}

func setupLargeObject(b *testing.B, sizeMB int) (*Bucket, string) {
	b.Helper()

	slash := filepath.ToSlash(b.TempDir())
	if slash == "" || slash[0] != '/' {
		slash = "/" + slash
	}

	u := url.URL{Scheme: "file", Path: slash, RawQuery: "create_dir=true"}

	bucket, err := Open(context.Background(), u.String())
	if err != nil {
		b.Fatalf("open bucket: %v", err)
	}

	data := make([]byte, sizeMB<<20)
	for i := range data {
		data[i] = byte(i)
	}

	const key = "kernel.bsp"

	if err := bucket.WriteAll(context.Background(), key, data, nil); err != nil {
		b.Fatalf("seed: %v", err)
	}

	return bucket, key
}

// BenchmarkReadAtStrategies compares one range read per ReadAt against the
// chunk-caching ReaderAt, at chunk sizes worth considering, over an
// SPK-shaped access pattern.
func BenchmarkReadAtStrategies(b *testing.B) {
	const sizeMB = 64

	bucket, key := setupLargeObject(b, sizeMB)

	const reads = 2000

	buf := make([]byte, 104)

	b.Run("plain-NewRangeReader-per-ReadAt", func(b *testing.B) {
		var calls atomic.Int64

		ra := plainReaderAt{bucket: bucket, key: key, calls: &calls}

		benchPattern(b, sizeMB, func(off int64) error {
			_, err := ra.ReadAt(buf, off)

			return err
		}, reads)
		b.ReportMetric(float64(calls.Load())/float64(b.N), "rangereads/op")
	})

	for _, chunk := range []int64{4 << 10, 64 << 10, 1 << 20} {
		b.Run(fmt.Sprintf("chunked-%dKiB", chunk>>10), func(b *testing.B) {
			ra, err := NewReaderAt(context.Background(), bucket, key,
				WithChunkSize(chunk), WithCachedChunks(16))
			if err != nil {
				b.Fatalf("NewReaderAt: %v", err)
			}

			defer func() { _ = ra.Close() }()

			b.ReportMetric(float64(chunk*16)/(1<<20), "MiB-resident")
			benchPattern(b, sizeMB, func(off int64) error {
				_, rerr := ra.ReadAt(buf, off)

				return rerr
			}, reads)
		})
	}
}
