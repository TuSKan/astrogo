package spk

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// RecordSize is the standard DAF record size in bytes.
const RecordSize = 1024

// FileRecord represents the DAF file record.
type FileRecord struct {
	Order  binary.ByteOrder
	IDWord uint64
	ND     int32
	NI     int32
	FWD    int32
	BWD    int32
	FREE   int32
}

// Summary represents a DAF segment summary.
type Summary struct {
	Doubles  []float64
	Integers []int32
}

// ReadAtCloser is an interface that combines io.ReaderAt and io.Closer.
type ReadAtCloser interface {
	io.ReaderAt
	io.Closer
}

// Reader provides tools to read DAF/SPK files.
//
// A *Reader is safe for concurrent use by multiple goroutines once
// constructed: FileRec is populated once in NewReader and never mutated
// afterward, every read method goes through F.ReadAt, whose io.ReaderAt
// contract requires position-independent, concurrency-safe reads regardless
// of the underlying implementation, and the record cache below is built for
// concurrent readers.
//
// A Reader from NewReader caches, for each segment it evaluates, the
// segment's directory and the last record it evaluated there (see
// recordCache). A Reader built as a literal has no cache and reads both on
// every evaluation; the results are the same to the last bit.
type Reader struct {
	F       ReadAtCloser
	FileRec FileRecord

	records *recordCache
}

// CacheDownload opens the SPK file named kernel, downloading it first when
// it is absent, into remote's registered cache directory for
// remote.NAIFSPK. Downloads are gated by remote's consent configuration —
// planetary kernels are large (de440s ≈ 32 MB, de440/de442 ≈ 115 MB,
// de441 parts multi-GB), and astrogo never downloads them without an
// explicit remote.EnableDownloads(maxSize, remote.NAIFSPK) call or a
// pre-seeded file.
//
// It provides an auto-healing mechanism for CI environments by automatically
// removing corrupt or truncated files. Integrity is checked three ways: a
// minimum-size floor derived from the DAF header, a structural parse of the
// summary/directory records (ReadSummaries), and a SHA-256 checksum recorded
// in a ".sha256" sidecar the first time the kernel is trusted and compared
// against on every later open — this last check is the only one that covers
// the bulk Chebyshev-coefficient data, which the first two never touch.
//
// If the file is incomplete or its metadata is invalid, the function:
//  1. Closes the file handle.
//  2. Removes the corrupt file from the filesystem.
//  3. Returns the error wrapped with a descriptive message.
func CacheDownload(ctx context.Context, kernel string) (*Reader, error) {
	fsys, key, err := remote.GetFile(ctx, remote.NAIFSPK, kernel, remote.WithCacheName(kernel))
	if err != nil {
		return nil, fmt.Errorf("jpl: SPK kernel %s: %w", kernel, err)
	}

	ra, err := remote.Open(ctx, fsys, key)
	if err != nil {
		return nil, fmt.Errorf("jpl: open SPK %s: %w", key, err)
	}

	r, err := NewReader(ra)
	if err != nil {
		return nil, discardIfCorrupt(ctx, fsys, key, ra.Close, err)
	}

	// Validate physical file size against DAF logical file length
	// FREE is the 1-based index of the first free double precision word.
	// Therefore, (FREE - 1) words * 8 bytes is the absolute minimum byte length.
	info, err := fs.Stat(fsys, key)
	if err != nil {
		closeErr := r.Close()

		return nil, errors.Join(fmt.Errorf("jpl: stat SPK %s: %w", key, err), closeErr)
	}

	size := info.Size()
	expectedMinSize := int64(r.FileRec.FREE-1) * 8

	if size < expectedMinSize {
		closeErr := r.Close()
		removeErr := remote.RemoveFile(ctx, fsys, key)

		return nil, errors.Join(
			fmt.Errorf("%w: truncated %d bytes, expected min %d bytes", ErrCorruptSPK, size, expectedMinSize),
			closeErr, removeErr,
		)
	}

	// Verify file integrity immediately to auto-heal CI pipelines
	if _, err := r.ReadSummaries(); err != nil {
		return nil, discardIfCorrupt(ctx, fsys, key, r.Close, err)
	}

	// ReadSummaries only parses the DAF directory/summary records, a small
	// fraction of the file — the bulk Chebyshev-coefficient data is never
	// touched by the checks above, so a bit flip there would go undetected.
	// NAIF does not publish per-kernel checksums to verify against, so we
	// record our own SHA-256 the first time a kernel is trusted and compare
	// against it on every later open of the same cached path. Hashing reads
	// through the already-open ra handle instead of opening the file again.
	if err := verifyOrBootstrapChecksum(ctx, fsys, key, ra, size); err != nil {
		// The sidecar goes with the kernel: keeping a checksum for a file
		// that is no longer there would make the next download's bootstrap
		// compare against a recording of the corrupt one.
		return nil, discardIfCorrupt(ctx, fsys, key, r.Close, err, func() error {
			return removeChecksumSidecar(ctx, fsys, key)
		})
	}

	return r, nil
}

// checksumSidecarKey returns the key used to persist a kernel's recorded
// SHA-256, alongside the kernel itself.
func checksumSidecarKey(key string) string { return key + ".sha256" }

// removeChecksumSidecar deletes a kernel's checksum sidecar, ignoring a
// missing one (nothing to clean up).
func removeChecksumSidecar(ctx context.Context, fsys remote.FS, key string) error {
	sumKey := checksumSidecarKey(key)

	if _, err := fs.Stat(fsys, sumKey); err != nil {
		// A failed check means we do not know whether a sidecar is there, not
		// that there is nothing to remove -- the comment here used to claim
		// the latter. Left alone either way: this runs after a checksum
		// mismatch has already been reported, the worst outcome is a stale
		// sidecar the next run re-checks, and failing the cleanup would
		// replace a real error with a bookkeeping one.
		return nil //nolint:nilerr // deliberate: see above
	}

	if err := remote.RemoveFile(ctx, fsys, sumKey); err != nil {
		return fmt.Errorf("jpl: checksum: remove sidecar: %w", err)
	}

	return nil
}

// verifyOrBootstrapChecksum compares a cached kernel's current SHA-256
// against the one recorded the last time it was trusted. If no sidecar
// exists yet (a fresh download, or a cache pre-dating this feature), the
// current hash is trusted and recorded for future opens instead of failing.
// Hashing reads through the already-open ra (a SectionReader over its
// io.ReaderAt) instead of opening the kernel a second time.
func verifyOrBootstrapChecksum(ctx context.Context, fsys remote.FS, key string, ra io.ReaderAt, size int64) error {
	h := sha256.New()
	if _, err := io.Copy(h, io.NewSectionReader(ra, 0, size)); err != nil {
		return fmt.Errorf("jpl: checksum: read: %w", err)
	}

	sum := hex.EncodeToString(h.Sum(nil))
	sumKey := checksumSidecarKey(key)

	recorded, found, err := readChecksumSidecar(fsys, sumKey)
	if err != nil {
		return err
	}

	if !found {
		writeErr := remote.WriteFile(ctx, fsys, sumKey, strings.NewReader(sum))
		if writeErr == nil {
			return nil
		}

		// Another process opening the same kernel may have recorded it in
		// the meantime, and on Windows its rename and this one exclude each
		// other. Its record is as good as this one's if it agrees, and is a
		// mismatch like any other if it does not.
		recorded, found, err = readChecksumSidecar(fsys, sumKey)
		if err != nil || !found {
			return fmt.Errorf("jpl: checksum: write sidecar: %w", writeErr)
		}
	}

	if recorded != sum {
		return fmt.Errorf("%w: sha256 mismatch (recorded %s, actual %s)", ErrCorruptSPK, recorded, sum)
	}

	return nil
}

// sidecarReadAttempts and sidecarRetryDelay bound how long a checksum sidecar
// that cannot be opened is waited out: three tries over 50 ms, as
// remote/file's readETag does for its own sidecar (#452).
const (
	sidecarReadAttempts = 3
	sidecarRetryDelay   = 25 * time.Millisecond
)

// readChecksumSidecar returns the hash recorded at sumKey, and whether one is
// recorded.
//
// Missing is final; any other failure is tried again. Several processes open
// one cached kernel at once — `go test ./...` runs a binary per package
// against a shared cache — and on Windows a sidecar being renamed into place
// by one of them cannot be opened by another: the rename holds delete access
// that Go's open does not share. It lasts milliseconds, and it failed the
// open of a kernel that was fine (#784).
func readChecksumSidecar(fsys remote.FS, sumKey string) (recorded string, found bool, err error) {
	for attempt := range sidecarReadAttempts {
		if attempt > 0 {
			time.Sleep(sidecarRetryDelay)
		}

		var b []byte

		b, err = fs.ReadFile(fsys, sumKey)
		if err == nil {
			return strings.TrimSpace(string(b)), true, nil
		}

		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
	}

	return "", false, fmt.Errorf("jpl: checksum: read sidecar: %w", err)
}

// NewReader opens a DAF/SPK file and reads its metadata.
func NewReader(f ReadAtCloser) (*Reader, error) {
	buf := make([]byte, RecordSize)
	if _, err := f.ReadAt(buf, 0); err != nil {
		// A short read at offset zero is the file being smaller than an
		// SPK's own file record, which no valid kernel can be — that is a
		// statement about the content, so it carries ErrCorruptSPK and a
		// cached copy is discarded. Any other read failure is the storage
		// misbehaving and says nothing about the bytes; see
		// discardIfCorrupt for why the difference decides whether a shared
		// 32 MB kernel gets deleted.
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w: file shorter than the %d-byte file record", ErrCorruptSPK, RecordSize)
		}

		return nil, fmt.Errorf("spk: read file record: %w", err)
	}

	format := string(buf[88:96])

	var order binary.ByteOrder = binary.LittleEndian
	if format == "BIG-IEEE" {
		order = binary.BigEndian
	}

	return &Reader{
		F: f,
		FileRec: FileRecord{
			IDWord: order.Uint64(buf[0:8]),
			ND:     int32(order.Uint32(buf[8:12])),
			NI:     int32(order.Uint32(buf[12:16])),
			FWD:    int32(order.Uint32(buf[76:80])),
			BWD:    int32(order.Uint32(buf[80:84])),
			FREE:   int32(order.Uint32(buf[84:88])),
			Order:  order,
		},
		records: new(recordCache),
	}, nil
}

// Close closes the file and drops the record cache, so a closed kernel
// answers no evaluation it would not have answered before: every one needs a
// read, and the read fails.
func (r *Reader) Close() error {
	r.records.clear()

	err := r.F.Close()
	if err != nil {
		return fmt.Errorf("spk: close: %w", err)
	}

	return nil
}

// maxSummaryRecords bounds how many summary records ReadSummaries will
// follow through the FWD linked list before giving up. Every legitimate
// SPK kernel this library encounters has at most a few hundred summary
// records; this cap is orders of magnitude more generous than that while
// still turning a self-referential or corrupted FWD chain into a bounded
// error instead of an unbounded append or an infinite loop. A cycle is
// also detected directly (via visited), independent of this cap.
const maxSummaryRecords = 1_000_000

// ReadSummaries reads all segment summaries.
func (r *Reader) ReadSummaries() ([]Summary, error) {
	var summaries []Summary

	nd, ni := int(r.FileRec.ND), int(r.FileRec.NI)
	if nd < 0 || ni < 0 || nd+ni == 0 {
		return nil, fmt.Errorf("%w: invalid summary shape ND=%d, NI=%d", ErrCorruptSPK, nd, ni)
	}

	// sumLen is derived from file-controlled ND/NI, then used below to slice
	// a fixed 1024-byte record buffer — bound it against the record size
	// itself (a summary that couldn't fit even one entry in its own
	// directory record is structurally invalid, regardless of file type)
	// rather than trusting it directly, which is what let a corrupted or
	// adversarial ND/NI drive a slice-bounds panic here before this check.
	sumLen := (nd + (ni+1)/2) * 8
	if sumLen <= 0 || sumLen > RecordSize-24 {
		return nil, fmt.Errorf("%w: invalid summary size %d bytes (from ND=%d, NI=%d)", ErrCorruptSPK, sumLen, nd, ni)
	}

	next := r.FileRec.FWD
	visited := make(map[int32]bool)

	for next != 0 {
		if next < 0 {
			return nil, fmt.Errorf("%w: negative FWD record pointer %d", ErrCorruptSPK, next)
		}

		if visited[next] {
			return nil, fmt.Errorf("%w: cyclic FWD chain revisits record %d", ErrCorruptSPK, next)
		}

		visited[next] = true
		if len(visited) > maxSummaryRecords {
			return nil, fmt.Errorf("%w: exceeded %d summary records following the FWD chain", ErrCorruptSPK, maxSummaryRecords)
		}

		buf := make([]byte, RecordSize)
		if _, err := r.F.ReadAt(buf, int64(next-1)*RecordSize); err != nil {
			return nil, fmt.Errorf("spk: read summary record: %w", err)
		}

		fwdFloat := math.Float64frombits(r.FileRec.Order.Uint64(buf[0:8]))
		if !finite(fwdFloat) {
			return nil, fmt.Errorf("%w: non-finite FWD pointer in summary record", ErrCorruptSPK)
		}

		fwd := int32(fwdFloat)

		nSumFloat := math.Float64frombits(r.FileRec.Order.Uint64(buf[16:24]))
		if !finite(nSumFloat) || nSumFloat < 0 {
			return nil, fmt.Errorf("%w: invalid summary count in summary record", ErrCorruptSPK)
		}

		nSum := int32(nSumFloat)
		if int64(24)+int64(nSum)*int64(sumLen) > RecordSize {
			return nil, fmt.Errorf("%w: %d summaries of %d bytes overflow the %d-byte record", ErrCorruptSPK, nSum, sumLen, RecordSize)
		}

		for i := range nSum {
			offset := 24 + int(i)*sumLen
			sumBuf := buf[offset : offset+sumLen]

			s := Summary{
				Doubles:  make([]float64, nd),
				Integers: make([]int32, ni),
			}

			for d := range nd {
				bits := r.FileRec.Order.Uint64(sumBuf[d*8 : (d+1)*8])
				s.Doubles[d] = math.Float64frombits(bits)
			}

			intStart := nd * 8
			for j := range ni {
				s.Integers[j] = int32(r.FileRec.Order.Uint32(sumBuf[intStart+j*4 : intStart+(j+1)*4]))
			}

			summaries = append(summaries, s)
		}

		next = fwd
	}

	return summaries, nil
}

// maxDoublesPerRead bounds a single ReadDoubles call to 16Mi doubles
// (128 MiB) — far more than any legitimate DAF record request needs (SPK
// segments read at most a few thousand doubles at a time), while turning a
// corrupted or adversarial word range into a bounded error instead of an
// attempted multi-gigabyte allocation. Computing count in int64 (rather
// than the file-controlled int32 startWord/endWord) also avoids the int32
// overflow that could otherwise make a huge count wrap negative and slip
// past a naive count<=0 check.
const maxDoublesPerRead = 16 << 20

// ReadDoubles reads a range of float64 from the data area.
func (r *Reader) ReadDoubles(startWord, endWord int32) ([]float64, error) {
	count := int64(endWord) - int64(startWord) + 1
	if count <= 0 || count > maxDoublesPerRead {
		return nil, fmt.Errorf("%w: %d to %d (count=%d)", ErrInvalidWordBounds, startWord, endWord, count)
	}

	buf := make([]byte, count*8)

	n, err := r.F.ReadAt(buf, (int64(startWord)-1)*8)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("spk: read doubles: %w", err)
	}

	if int64(n) < int64(len(buf)) && (errors.Is(err, io.EOF) || err == nil) {
		return nil, fmt.Errorf("%w: unexpected EOF reading word %d", ErrCorruptSPK, startWord)
	}

	res := make([]float64, count)
	for i := range count {
		bits := r.FileRec.Order.Uint64(buf[i*8 : (i+1)*8])
		res[i] = math.Float64frombits(bits)
	}

	return res, nil
}

// finite reports whether v is neither NaN nor ±Inf — used throughout this
// file to reject a corrupted or adversarial kernel's non-finite metadata
// (record counts, step sizes, epochs) before it can propagate into a
// slice index or division.
func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// Segment represents an SPK segment descriptor.
type Segment struct {
	Target, Center, Frame, Type int32
	StartET, EndET              float64
	StartAddr, EndAddr          int32
}

// SelectSegment finds the highest priority segment for target and ET.
func SelectSegment(segments []Segment, targetID int32, et float64) (*Segment, error) {
	for _, v := range slices.Backward(segments) {
		s := &v
		if s.Target == targetID && et >= s.StartET && et <= s.EndET {
			return s, nil
		}
	}

	return nil, fmt.Errorf("%w: %d at ET %f", ErrNoCoverage, targetID, et)
}

// EvaluateSegment computes state from an SPK segment.
func EvaluateSegment(s *Segment, r *Reader, et float64) (pos, vel vector.Vec3, err error) {
	switch s.Type {
	case 2:
		return evaluateType2(s, r, et)
	case 3:
		return evaluateType3(s, r, et)
	case 21:
		return evaluateType21(s, r, et)
	default:
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: %d", ErrUnsupportedSegment, s.Type)
	}
}

// maxAllowedDim is the largest per-component difference-table size (MAXDIM)
// this evaluator accepts — matches SPICE's own MAXTRM constant for type 21
// (spk21.inc), also adopted independently by whiskie14142/spktype21 (a
// from-scratch reimplementation used to cross-check the algorithm below).
const maxAllowedDim = 25

// evaluateType21 evaluates a type 21 (Extended Modified Difference Arrays)
// segment. The on-disk layout — confirmed against real Horizons-generated
// small-body kernels, not just the SPK Required Reading text, which
// describes an ordering that doesn't match actual data (the same
// discrepancy independently documented by whiskie14142/spktype21, a
// from-scratch Python port this implementation was cross-checked against):
//
//	[ nRecs records, each DLSIZE = 4*MAXDIM+11 doubles ]
//	[ nRecs epoch values, one per record                ]
//	[ optional epoch directory (nRecs/100 doubles) — a search accelerator
//	  for huge segments, skipped here in favor of a direct search over the
//	  epoch table, which is correct regardless of segment size            ]
//	[ MAXDIM  (a single double, at EndAddr-1)            ]
//	[ nRecs   (a single double, at EndAddr)              ]
//
// Unlike type 1 (which fixes MAXDIM at 15), type 21's whole point is a
// variable, per-segment difference-table size, so it must be read from the
// segment itself rather than assumed.
//
// Each record itself is laid out as:
//
//	t0 (1) | G stepsize vector (MAXDIM) | (refPos,refVel) interleaved per
//	axis, x,y,z (6) | MDA divided-difference table, grouped by axis
//	(3*MAXDIM) | KQMAX1 (1) | per-axis integration order KQ[3]
//
// and the position/velocity extrapolation itself follows SPICE's spke21_
// algorithm: build FC/WC coefficient arrays from the stepsize vector, then
// recursively build the W(k) weights used to sum each axis's difference
// table up to that axis's own order KQ[axis] (not one order shared across
// all three axes).
func evaluateType21(s *Segment, r *Reader, et float64) (pos, vel vector.Vec3, err error) {
	dir, entry, err := r.type21Directory(s)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	// The smallest index whose epoch exceeds et is the record whose
	// difference line is valid there — used directly, with no -1
	// adjustment (confirmed against the reference implementation: it
	// keeps the found 1-based loop index as-is for the record number,
	// which is this same 0-based search index here).
	idx := int32(sort.Search(int(dir.nRecs), func(i int) bool {
		return dir.epochs[i] > et
	}))
	if idx >= dir.nRecs {
		idx = dir.nRecs - 1
	}

	rec, err := r.type21Record(s, dir, entry, idx)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	maxDim := dir.maxDim
	t0 := rec[0]
	g := rec[1 : maxDim+1]
	refPos := [3]float64{rec[maxDim+1], rec[maxDim+3], rec[maxDim+5]}
	refVel := [3]float64{rec[maxDim+2], rec[maxDim+4], rec[maxDim+6]}
	dt := rec[maxDim+7 : maxDim+7+3*maxDim] // column-major: axis i occupies dt[i*maxDim : (i+1)*maxDim]

	kqmax1 := int32(rec[4*maxDim+7])
	kq := [3]int32{int32(rec[4*maxDim+8]), int32(rec[4*maxDim+9]), int32(rec[4*maxDim+10])}

	delta := et - t0

	// fc/wc/w are sized generously beyond any index the recursion below
	// can reach (bounded by kqmax1 <= maxAllowedDim+1) — cheap, fixed-size
	// stack arrays, not a tight/fragile bound.
	const arrSize = 2 * (maxAllowedDim + 2)

	var fc, w [arrSize]float64

	var wc [arrSize]float64

	mq2 := kqmax1 - 2
	tp := delta

	for j := int32(1); j <= mq2; j++ {
		if g[j-1] == 0 {
			return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: zero step size at index %d", ErrInvalidOrder, j)
		}

		fc[j] = tp / g[j-1]
		wc[j-1] = delta / g[j-1]
		tp = delta + g[j-1]
	}

	for j := int32(1); j <= kqmax1; j++ {
		w[j-1] = 1.0 / float64(j)
	}

	ks := kqmax1 - 1
	ks1 := ks - 1
	jx := int32(0)

	for ks >= 2 {
		jx++

		for j := int32(1); j <= jx; j++ {
			w[j+ks-1] = fc[j]*w[j+ks1-1] - wc[j-1]*w[j+ks-1]
		}

		ks = ks1
		ks1--
	}

	axisSum := func(axis int32) float64 {
		sum := 0.0
		for j := kq[axis]; j >= 1; j-- {
			sum += dt[axis*maxDim+j-1] * w[j+ks-1]
		}

		return sum
	}

	var posArr [3]float64
	for i := range int32(3) {
		posArr[i] = refPos[i] + delta*(refVel[i]+delta*axisSum(i))
	}

	for j := int32(1); j <= jx; j++ {
		w[j+ks-1] = fc[j]*w[j+ks1-1] - wc[j-1]*w[j+ks-1]
	}

	ks--

	var velArr [3]float64
	for i := range int32(3) {
		velArr[i] = refVel[i] + delta*axisSum(i)
	}

	pos = vector.Vec3{X: posArr[0], Y: posArr[1], Z: posArr[2]}
	vel = vector.Vec3{X: velArr[0], Y: velArr[1], Z: velArr[2]}

	if !finite(pos.X) || !finite(pos.Y) || !finite(pos.Z) || !finite(vel.X) || !finite(vel.Y) || !finite(vel.Z) {
		// A non-finite step size (g) not already caught by the zero check
		// above, or other corrupted difference-table data, can still
		// propagate a NaN/Inf through the FC/WC/W recursion — fail loudly
		// here rather than hand a silently-bad state/velocity to a caller.
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: non-finite evaluated position/velocity", ErrCorruptSPK)
	}

	return pos, vel, nil
}

// validateChebyshevMeta checks the (tInit, tLen, rSize) triple
// evaluateType2/evaluateType3 derive their record layout from, before that
// layout is used to size any read, slice, or division. A corrupt kernel
// could otherwise supply a non-finite or non-positive tLen (dividing to
// ±Inf/NaN in the idx computation below), or an rSize too small to hold
// even the leading mid/radius pair — evaluateType2/3 used to read rec[0]
// and rec[1] unconditionally, so a too-short record panicked instead of
// erroring.
func validateChebyshevMeta(tInit, tLen float64, rSize int32) error {
	if !finite(tInit) || !finite(tLen) || tLen <= 0 {
		return fmt.Errorf("%w: non-finite or non-positive record length %g", ErrCorruptSPK, tLen)
	}

	if rSize < 2 {
		return fmt.Errorf("%w: record size %d too small (need >= 2)", ErrRecordTooShort, rSize)
	}

	return nil
}

func evaluateType2(s *Segment, r *Reader, et float64) (pos, vel vector.Vec3, err error) {
	rec, nCoeffs, tau, err := r.chebyshevWords(s, et, 3)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	radius := rec[1]
	pos.X, vel.X = EvalChebyshev(rec[2:2+nCoeffs], tau, radius, true)
	pos.Y, vel.Y = EvalChebyshev(rec[2+nCoeffs:2+2*nCoeffs], tau, radius, true)
	pos.Z, vel.Z = EvalChebyshev(rec[2+2*nCoeffs:2+3*nCoeffs], tau, radius, true)

	if !finite(pos.X) || !finite(pos.Y) || !finite(pos.Z) || !finite(vel.X) || !finite(vel.Y) || !finite(vel.Z) {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: non-finite evaluated position/velocity", ErrCorruptSPK)
	}

	return pos, vel, nil
}

func evaluateType3(s *Segment, r *Reader, et float64) (pos, vel vector.Vec3, err error) {
	rec, nCoeffs, tau, err := r.chebyshevWords(s, et, 6)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	radius := rec[1]
	pos.X, _ = EvalChebyshev(rec[2:2+nCoeffs], tau, radius, false)
	pos.Y, _ = EvalChebyshev(rec[2+nCoeffs:2+2*nCoeffs], tau, radius, false)
	pos.Z, _ = EvalChebyshev(rec[2+2*nCoeffs:2+3*nCoeffs], tau, radius, false)
	vStart := 2 + 3*nCoeffs
	vel.X, _ = EvalChebyshev(rec[vStart:vStart+nCoeffs], tau, radius, false)
	vel.Y, _ = EvalChebyshev(rec[vStart+nCoeffs:vStart+2*nCoeffs], tau, radius, false)
	vel.Z, _ = EvalChebyshev(rec[vStart+2*nCoeffs:vStart+3*nCoeffs], tau, radius, false)

	if !finite(pos.X) || !finite(pos.Y) || !finite(pos.Z) || !finite(vel.X) || !finite(vel.Y) || !finite(vel.Z) {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: non-finite evaluated position/velocity", ErrCorruptSPK)
	}

	return pos, vel, nil
}

// recordTau is et's place on its record's interval, mid ± radius, where the
// record's Chebyshev series is defined: τ in [−1, 1].
//
// A record's polynomial fits its own interval and nothing beyond it; outside,
// the series grows without bound. The final instant of a segment, where
// (et − INIT)/INTLEN is exactly N, belongs to the last record at τ = 1 (#807),
// but that index covers the whole interval after it as well, and an epoch
// there used to be evaluated on the last record's polynomial at τ up to 3 —
// forty times the true value a record past the end, with no error (#815).
// The record's own mid and radius are the one test that a directory, a
// segment window or a caller cannot get wrong for it.
//
// The allowance is the rounding of the two computations that put et here, as
// a fraction of the half-span:
//
//   - the record's choice. The directory's index ⌊(et − INIT)/INTLEN⌋ rounds
//     et − INIT to an ulp, so an epoch that close before a boundary goes to
//     the record that opens there, a hair outside it. On de440s, which starts
//     in 1849, that ulp is about a microsecond: an epoch 2⁻⁴⁰ of a record
//     before a boundary went to the next record at τ = −1 − 2⁻³⁹ —
//     measured, not supposed: TestEveryRecordBoundaryIsInsideItsRecord found
//     it. Evaluating a polynomial that far past its end is exact to its own
//     precision, and is what any SPK reader does there.
//   - τ itself: et and the stored mid to an ulp each, and the division.
//
// At the record boundaries themselves the allowance is never needed: across
// de440, de440s, de441, de442 and nep097, about 426,000 boundary
// evaluations, |τ| never exceeds 1, because those kernels' middles and
// half-spans are exact.
func recordTau(et, mid, radius, tInit float64) (float64, error) {
	tau := (et - mid) / radius

	allowance := (2*ulp(et-tInit)+ulp(mid)+ulp(et))/math.Abs(radius) + 4*epsilon
	if !(math.Abs(tau) <= 1+allowance) {
		return 0, fmt.Errorf("%w: ET %v is at τ = %.6g on its record, whose polynomial holds only for |τ| <= 1",
			ErrCorruptSPK, et, tau)
	}

	return tau, nil
}

// epsilon is the spacing of float64 values at 1.
const epsilon = 0x1p-52

// ulp is the spacing of float64 values at |x|.
func ulp(x float64) float64 {
	x = math.Abs(x)

	return math.Nextafter(x, math.Inf(1)) - x
}

// chebyshevDir is a type 2 or 3 segment's directory, validated: records of
// tLen seconds each from tInit, nRecs of them, each rSize words holding
// nCoeffs coefficients for each of the record's components.
type chebyshevDir struct {
	tInit, tLen           float64
	rSize, nCoeffs, nRecs int32
}

// chebyshevDirectory is s's four-word directory (INIT, INTLEN, RSIZE, N),
// read and validated on first use and kept by the record cache after.
// components is 3 for type 2, position only, and 6 for type 3.
//
// A directory that fails validation is never kept, so it fails the same way
// on every call, as it always has.
func (r *Reader) chebyshevDirectory(s *Segment, components int32) (*chebyshevDir, *segmentEntry, error) {
	if e := r.records.load(s); e != nil {
		return e.cheb, e, nil
	}

	meta, err := r.ReadDoubles(s.EndAddr-3, s.EndAddr)
	if err != nil {
		return nil, nil, err
	}

	tInit, tLen, rSize := meta[0], meta[1], int32(meta[2])
	if err := validateChebyshevMeta(tInit, tLen, rSize); err != nil {
		return nil, nil, err
	}

	nCoeffs := (rSize - 2) / components
	if nCoeffs <= 0 {
		return nil, nil, fmt.Errorf("%w: %d coefficients derived from record size %d", ErrRecordTooShort, nCoeffs, rSize)
	}

	n := meta[3]
	if !finite(n) || n < 1 || n != math.Trunc(n) || n > math.MaxInt32 {
		return nil, nil, fmt.Errorf("%w: %v records", ErrInvalidRecordCount, n)
	}

	dir := &chebyshevDir{tInit: tInit, tLen: tLen, rSize: rSize, nCoeffs: nCoeffs, nRecs: int32(n)}

	return dir, r.records.store(s, &segmentEntry{cheb: dir}), nil
}

// record is the first word and the index of the record that holds et.
//
// An et at the end of the last record is the segment's final instant, which
// segment selection admits, and it belongs to the last record: the index
// (et−tInit)/tLen is then exactly n, one past the end. Unclamped, that read
// the directory and whatever follows it as Chebyshev coefficients — finite
// numbers, so no check caught them, and de440s put the Earth 6×10⁶¹ km from
// its center at the last instant the kernel covers (#807). jplephem maps
// index n to the last record the same way. Any other index outside the
// records means the directory does not describe et, and is reported rather
// than clamped: a clamp would hand back the nearest record's polynomial far
// outside the interval it fits. Index n covers the whole interval after the
// final instant, not only the instant; an epoch there reaches the last record
// and is refused by recordTau (#815).
func (d *chebyshevDir) record(s *Segment, et float64) (start, idx int32, err error) {
	pos := math.Floor((et - d.tInit) / d.tLen)
	if !(pos >= 0 && pos <= float64(d.nRecs)) {
		return 0, 0, fmt.Errorf("%w: ET %v falls in record %v of a segment with %d", ErrCorruptSPK, et, pos, d.nRecs)
	}

	idx = int32(pos)
	if idx == d.nRecs {
		idx--
	}

	// The records end where the directory's four words begin. Checking that
	// here, rather than trusting N, keeps a directory that overstates its
	// record count from steering the read into whatever follows.
	start64 := int64(s.StartAddr) + int64(idx)*int64(d.rSize)
	if start64+int64(d.rSize)-1 > int64(s.EndAddr)-4 {
		return 0, 0, fmt.Errorf("%w: record %d of %d words runs past the segment's records", ErrCorruptSPK, idx, d.rSize)
	}

	return int32(start64), idx, nil
}

// chebyshevWords is the record of s that holds et, validated, its
// coefficients per component, and et's place τ on it: the record from the
// record cache when the last record evaluated in s is that one, read and
// decoded otherwise.
func (r *Reader) chebyshevWords(s *Segment, et float64, components int32) (rec []float64, nCoeffs int32, tau float64, err error) {
	dir, entry, err := r.chebyshevDirectory(s, components)
	if err != nil {
		return nil, 0, 0, err
	}

	start, idx, err := dir.record(s, et)
	if err != nil {
		return nil, 0, 0, err
	}

	rec = entry.cached(idx)
	if rec == nil {
		rec, err = r.ReadDoubles(start, start+dir.rSize-1)
		if err != nil {
			return nil, 0, 0, err
		}

		if int32(len(rec)) < 2+components*dir.nCoeffs {
			return nil, 0, 0, fmt.Errorf("%w: %d doubles (need >= %d)", ErrRecordTooShort, len(rec), 2+components*dir.nCoeffs)
		}

		mid, radius := rec[0], rec[1]
		if !finite(mid) || !finite(radius) || radius == 0 {
			return nil, 0, 0, fmt.Errorf("%w: invalid mid=%g/radius=%g", ErrCorruptSPK, mid, radius)
		}

		entry.keep(idx, rec)
	}

	tau, err = recordTau(et, rec[0], rec[1], dir.tInit)
	if err != nil {
		return nil, 0, 0, err
	}

	return rec, dir.nCoeffs, tau, nil
}

// type21Dir is a type 21 segment's directory, validated: MAXDIM, the record
// count, the record size MAXDIM gives, and the epoch table.
type type21Dir struct {
	maxDim, nRecs, dlSize int32
	epochs                []float64
}

// type21Directory is s's directory and epoch table, read and validated on
// first use and kept by the record cache after.
//
// The epoch table is nRecs doubles. Every evaluation used to read all of it,
// allocating it twice, to binary-search one epoch; held once it costs the same
// memory a single evaluation used to allocate.
func (r *Reader) type21Directory(s *Segment) (*type21Dir, *segmentEntry, error) {
	if e := r.records.load(s); e != nil {
		return e.t21, e, nil
	}

	meta, err := r.ReadDoubles(s.EndAddr-1, s.EndAddr)
	if err != nil {
		return nil, nil, err
	}

	if !finite(meta[0]) || !finite(meta[1]) {
		return nil, nil, fmt.Errorf("%w: non-finite MAXDIM/nRecs metadata", ErrCorruptSPK)
	}

	maxDim := int32(meta[0])
	nRecs := int32(meta[1])

	// maxType21Records is a generous upper bound guarding nRecs*dlSize
	// (below) against int32 overflow — no real Horizons-generated small-body
	// kernel this library targets has anywhere near this many records.
	const maxType21Records = 10_000_000
	if nRecs <= 0 || nRecs > maxType21Records {
		return nil, nil, fmt.Errorf("%w: %d", ErrInvalidRecordCount, nRecs)
	}

	if maxDim <= 0 || maxDim > maxAllowedDim {
		return nil, nil, fmt.Errorf("%w: MAXDIM=%d, allowed [1,%d]", ErrInvalidOrder, maxDim, maxAllowedDim)
	}

	dlSize := 4*maxDim + 11

	epochBase := s.StartAddr + nRecs*dlSize

	epochs, err := r.ReadDoubles(epochBase, epochBase+nRecs-1)
	if err != nil {
		return nil, nil, err
	}

	dir := &type21Dir{maxDim: maxDim, nRecs: nRecs, dlSize: dlSize, epochs: epochs}

	return dir, r.records.store(s, &segmentEntry{t21: dir}), nil
}

// type21Record is record idx of s, validated: from the record cache when it
// is the last one evaluated in s, read and checked otherwise.
func (r *Reader) type21Record(s *Segment, dir *type21Dir, entry *segmentEntry, idx int32) ([]float64, error) {
	if rec := entry.cached(idx); rec != nil {
		return rec, nil
	}

	maxDim := dir.maxDim
	recStart := s.StartAddr + idx*dir.dlSize

	rec, err := r.ReadDoubles(recStart, recStart+dir.dlSize-1)
	if err != nil {
		return nil, err
	}

	if int32(len(rec)) < dir.dlSize {
		return nil, fmt.Errorf("%w: %d doubles (need >= %d)", ErrRecordTooShort, len(rec), dir.dlSize)
	}

	if !finite(rec[4*maxDim+7]) || !finite(rec[4*maxDim+8]) || !finite(rec[4*maxDim+9]) || !finite(rec[4*maxDim+10]) {
		return nil, fmt.Errorf("%w: non-finite KQMAX1/KQ metadata", ErrCorruptSPK)
	}

	kqmax1 := int32(rec[4*maxDim+7])
	kq := [3]int32{int32(rec[4*maxDim+8]), int32(rec[4*maxDim+9]), int32(rec[4*maxDim+10])}

	if kqmax1 < 2 || kqmax1 > maxAllowedDim+1 {
		return nil, fmt.Errorf("%w: KQMAX1=%d, allowed [2,%d]", ErrInvalidOrder, kqmax1, maxAllowedDim+1)
	}

	// KQMAX1/KQ are read straight from the record with no guarantee they
	// stay within this segment's own MAXDIM — unlike the maxAllowedDim+1
	// check above (a fixed library-wide ceiling), this is the check that a
	// small, valid-looking MAXDIM (e.g. 1) can't be paired with a
	// KQMAX1/KQ large enough to index g/dt/w out of their MAXDIM-sized
	// bounds in evaluateType21.
	if kqmax1 > maxDim+1 {
		return nil, fmt.Errorf("%w: KQMAX1=%d exceeds this segment's own MAXDIM=%d", ErrInvalidOrder, kqmax1, maxDim)
	}

	for axis, k := range kq {
		if k < 0 || k > maxDim {
			return nil, fmt.Errorf("%w: KQ[%d]=%d exceeds MAXDIM=%d", ErrInvalidOrder, axis, k, maxDim)
		}
	}

	entry.keep(idx, rec)

	return rec, nil
}

// recordCache is what a Reader keeps of the segments it has evaluated: each
// one's directory, read and validated once, and the last record evaluated in
// it, decoded (#802).
//
// # Why records, not bytes
//
// remote.Open's chunk cache holds 16 chunks of 64 KiB. One body's evaluation
// touches two places in the file — its segment's directory, at the segment's
// end, and the record in force — so it stays warm; a sky does not. Nine
// bodies at one instant touch the directory and the record of each of 13
// segments, 26 places, and 18 of each sky's 60 reads missed and reloaded a
// chunk: the geometric sky from the kernel on disk took 360 µs, 23 times the
// same evaluations from memory (docs/ephemeris.md §4). A bigger chunk cache
// only moves the point where it thrashes. What a caller evaluates again is a
// record: a planetary record spans days, so a light-time iteration, a scan's
// next step and the next body asking for the Earth all land in the one just
// used. CSPICE buffers segment records for the same reason.
//
// # Why it is safe
//
// The arithmetic is untouched: a cached record is the same []float64 the
// read would return, so every evaluation is bit-identical to an uncached one,
// which a Reader built as a literal still performs. Nothing is cached until it
// has passed the validation a read would apply, so a corrupt directory or
// record fails on every call as before. Entries are keyed by the segment's
// addresses and type rather than its pointer, since SelectSegment and callers
// hand back copies; a record is immutable once published through an
// atomic.Pointer, so concurrent readers share it without a lock, and two that
// miss together each read it and publish an identical one.
type recordCache struct {
	segments sync.Map // segmentKey → *segmentEntry
}

type segmentKey struct {
	start, end, typ int32
}

// segmentEntry is one segment's cached directory — cheb for types 2 and 3,
// t21 for type 21 — and its last evaluated record.
type segmentEntry struct {
	cheb *chebyshevDir
	t21  *type21Dir
	last atomic.Pointer[cachedRecord]
}

type cachedRecord struct {
	idx   int32
	words []float64
}

func keyOf(s *Segment) segmentKey {
	return segmentKey{start: s.StartAddr, end: s.EndAddr, typ: s.Type}
}

// load is s's entry, or nil when there is none or no cache.
func (c *recordCache) load(s *Segment) *segmentEntry {
	if c == nil {
		return nil
	}

	v, ok := c.segments.Load(keyOf(s))
	if !ok {
		return nil
	}

	e, _ := v.(*segmentEntry)

	return e
}

// store keeps e for s unless another reader kept one first, and returns the
// entry in force; with no cache it keeps nothing and returns nil.
func (c *recordCache) store(s *Segment, e *segmentEntry) *segmentEntry {
	if c == nil {
		return nil
	}

	v, _ := c.segments.LoadOrStore(keyOf(s), e)
	got, _ := v.(*segmentEntry)

	return got
}

func (c *recordCache) clear() {
	if c != nil {
		c.segments.Clear()
	}
}

// cached is record idx when it is the last one evaluated in e's segment.
func (e *segmentEntry) cached(idx int32) []float64 {
	if e == nil {
		return nil
	}

	if rec := e.last.Load(); rec != nil && rec.idx == idx {
		return rec.words
	}

	return nil
}

// keep makes record idx the last one evaluated in e's segment.
func (e *segmentEntry) keep(idx int32, words []float64) {
	if e != nil {
		e.last.Store(&cachedRecord{idx: idx, words: words})
	}
}

// EvalChebyshev evaluates a Chebyshev polynomial and optionally its derivative.
func EvalChebyshev(coeffs []float64, tau, radius float64, calcDeriv bool) (p, v float64) {
	n := len(coeffs)
	if n == 0 {
		return 0, 0
	}

	if n == 1 {
		return coeffs[0], 0
	}

	t0, t1 := 1.0, tau
	p = coeffs[0]*t0 + coeffs[1]*t1

	var u0, u1 float64
	if calcDeriv {
		u0, u1 = 0.0, 1.0
		v = coeffs[1] * u1
	}

	for i := 2; i < n; i++ {
		tn := 2.0*tau*t1 - t0
		if calcDeriv {
			un := 2.0*tau*u1 - u0 + 2.0*t1
			v += coeffs[i] * un
			u0, u1 = u1, un
		}

		p += coeffs[i] * tn
		t0, t1 = t1, tn
	}

	if calcDeriv {
		v /= radius
	}

	return p, v
}

// discardIfCorrupt is the one place that decides whether a kernel that failed
// to open should be deleted from the cache.
//
// # Why it is a decision and not a cleanup
//
// Opening a cached kernel can fail for two entirely different reasons, and
// they want opposite responses:
//
//   - The bytes are wrong — a checksum that computed cleanly and did not
//     match, a DAF structure that parsed and was invalid, a file shorter than
//     its own header says it is. The cached copy is worthless and deleting it
//     is what makes the next run heal itself. Every one of these wraps
//     [ErrCorruptSPK].
//   - The bytes could not be read — "Access is denied" because another
//     process holds the handle, a short read, a canceled context, a full
//     disk. The file is very likely fine and the condition is transient.
//
// These used to be the same branch, so the second deleted the file. That is
// worse than merely wrong, because the cache is shared: `go test ./...` runs
// packages as separate processes against one cache directory, and one of them
// losing a race deleted the 32 MB kernel out from under the others, which then
// spent three minutes each failing to find a file that had been there. It is
// the observed cause of intermittent Windows CI failures on main, and a user
// running a scheduler alongside an analysis script hits the same thing with a
// 3 GB DE441 part.
//
// So an I/O failure now returns and leaves the file alone. The next open tries
// again, which is exactly what a transient condition wants. Only a proven
// content failure is destructive.
//
// The handle is closed either way. extra runs only when the file is deleted —
// it is for artefacts that describe the kernel and must not outlive it.
func discardIfCorrupt(ctx context.Context, fsys remote.FS, key string,
	closeFile func() error, cause error, extra ...func() error,
) error {
	closeErr := closeFile()

	if !errors.Is(cause, ErrCorruptSPK) {
		return errors.Join(fmt.Errorf("jpl: open SPK %s: %w", key, cause), closeErr)
	}

	errs := []error{
		fmt.Errorf("jpl: corrupt SPK file gracefully deleted: %w", cause),
		closeErr,
		remote.RemoveFile(ctx, fsys, key),
	}

	for _, fn := range extra {
		errs = append(errs, fn())
	}

	return errors.Join(errs...)
}
