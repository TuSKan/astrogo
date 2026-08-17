package spk

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sort"
	"strings"

	"gocloud.dev/gcerrors"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/file"
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
// afterward, and every read method goes through F.ReadAt, whose io.ReaderAt
// contract requires position-independent, concurrency-safe reads regardless
// of the underlying implementation.
type Reader struct {
	F       ReadAtCloser
	FileRec FileRecord
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
	bucket, key, err := remote.GetFile(ctx, remote.NAIFSPK, kernel, remote.WithCacheName(kernel))
	if err != nil {
		return nil, fmt.Errorf("jpl: SPK kernel %s: %w", kernel, err)
	}

	ra, err := file.NewReaderAt(ctx, bucket, key)
	if err != nil {
		return nil, fmt.Errorf("jpl: open SPK %s: %w", key, err)
	}

	r, err := NewReader(ra)
	if err != nil {
		closeErr := ra.Close()
		removeErr := bucket.Delete(ctx, key)

		return nil, errors.Join(err, closeErr, removeErr)
	}

	// Validate physical file size against DAF logical file length
	// FREE is the 1-based index of the first free double precision word.
	// Therefore, (FREE - 1) words * 8 bytes is the absolute minimum byte length.
	attrs, err := bucket.Attributes(ctx, key)
	if err != nil {
		closeErr := r.Close()

		return nil, errors.Join(fmt.Errorf("jpl: stat SPK %s: %w", key, err), closeErr)
	}

	size := attrs.Size
	expectedMinSize := int64(r.FileRec.FREE-1) * 8

	if size < expectedMinSize {
		closeErr := r.Close()
		removeErr := bucket.Delete(ctx, key)

		return nil, errors.Join(
			fmt.Errorf("%w: truncated %d bytes, expected min %d bytes", ErrCorruptSPK, size, expectedMinSize),
			closeErr, removeErr,
		)
	}

	// Verify file integrity immediately to auto-heal CI pipelines
	if _, err := r.ReadSummaries(); err != nil {
		closeErr := r.Close()
		removeErr := bucket.Delete(ctx, key)

		return nil, errors.Join(fmt.Errorf("jpl: corrupt SPK file gracefully deleted: %w", err), closeErr, removeErr)
	}

	// ReadSummaries only parses the DAF directory/summary records, a small
	// fraction of the file — the bulk Chebyshev-coefficient data is never
	// touched by the checks above, so a bit flip there would go undetected.
	// NAIF does not publish per-kernel checksums to verify against, so we
	// record our own SHA-256 the first time a kernel is trusted and compare
	// against it on every later open of the same cached path. Hashing reads
	// through the already-open ra handle instead of opening the file again.
	if err := verifyOrBootstrapChecksum(ctx, bucket, key, ra, size); err != nil {
		closeErr := r.Close()
		removeErr := bucket.Delete(ctx, key)
		sumRemoveErr := removeChecksumSidecar(ctx, bucket, key)

		return nil, errors.Join(fmt.Errorf("jpl: corrupt SPK file gracefully deleted: %w", err), closeErr, removeErr, sumRemoveErr)
	}

	return r, nil
}

// checksumSidecarKey returns the key used to persist a kernel's recorded
// SHA-256, alongside the kernel itself.
func checksumSidecarKey(key string) string { return key + ".sha256" }

// removeChecksumSidecar deletes a kernel's checksum sidecar, ignoring a
// missing one (nothing to clean up).
func removeChecksumSidecar(ctx context.Context, bucket *file.Bucket, key string) error {
	sumKey := checksumSidecarKey(key)

	if exists, err := bucket.Exists(ctx, sumKey); err != nil || !exists {
		return nil //nolint:nilerr // a failed existence check just means "nothing to clean up here either"
	}

	if err := bucket.Delete(ctx, sumKey); err != nil {
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
func verifyOrBootstrapChecksum(ctx context.Context, bucket *file.Bucket, key string, ra io.ReaderAt, size int64) error {
	h := sha256.New()
	if _, err := io.Copy(h, io.NewSectionReader(ra, 0, size)); err != nil {
		return fmt.Errorf("jpl: checksum: read: %w", err)
	}

	sum := hex.EncodeToString(h.Sum(nil))
	sumKey := checksumSidecarKey(key)

	existing, err := bucket.ReadAll(ctx, sumKey)
	if err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			if err := file.Save(ctx, bucket, sumKey, strings.NewReader(sum)); err != nil {
				return fmt.Errorf("jpl: checksum: write sidecar: %w", err)
			}

			return nil
		}

		return fmt.Errorf("jpl: checksum: read sidecar: %w", err)
	}

	if strings.TrimSpace(string(existing)) != sum {
		return fmt.Errorf("%w: sha256 mismatch (recorded %s, actual %s)", ErrCorruptSPK, strings.TrimSpace(string(existing)), sum)
	}

	return nil
}

// NewReader opens a DAF/SPK file and reads its metadata.
func NewReader(f ReadAtCloser) (*Reader, error) {
	buf := make([]byte, RecordSize)
	if _, err := f.ReadAt(buf, 0); err != nil {
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
	}, nil
}

// Close closes the file.
func (r *Reader) Close() error {
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
	meta, err := r.ReadDoubles(s.EndAddr-1, s.EndAddr)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	if !finite(meta[0]) || !finite(meta[1]) {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: non-finite MAXDIM/nRecs metadata", ErrCorruptSPK)
	}

	maxDim := int32(meta[0])
	nRecs := int32(meta[1])

	// maxType21Records is a generous upper bound guarding nRecs*dlSize
	// (below) against int32 overflow — no real Horizons-generated small-body
	// kernel this library targets has anywhere near this many records.
	const maxType21Records = 10_000_000
	if nRecs <= 0 || nRecs > maxType21Records {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: %d", ErrInvalidRecordCount, nRecs)
	}

	if maxDim <= 0 || maxDim > maxAllowedDim {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: MAXDIM=%d, allowed [1,%d]", ErrInvalidOrder, maxDim, maxAllowedDim)
	}

	dlSize := 4*maxDim + 11

	epochBase := s.StartAddr + nRecs*dlSize

	epochs, err := r.ReadDoubles(epochBase, epochBase+nRecs-1)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	// The smallest index whose epoch exceeds et is the record whose
	// difference line is valid there — used directly, with no -1
	// adjustment (confirmed against the reference implementation: it
	// keeps the found 1-based loop index as-is for the record number,
	// which is this same 0-based search index here).
	idx := int32(sort.Search(int(nRecs), func(i int) bool {
		return epochs[i] > et
	}))
	if idx >= nRecs {
		idx = nRecs - 1
	}

	recStart := s.StartAddr + idx*dlSize

	rec, err := r.ReadDoubles(recStart, recStart+dlSize-1)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	if int32(len(rec)) < dlSize {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: %d doubles (need >= %d)", ErrRecordTooShort, len(rec), dlSize)
	}

	t0 := rec[0]
	g := rec[1 : maxDim+1]
	refPos := [3]float64{rec[maxDim+1], rec[maxDim+3], rec[maxDim+5]}
	refVel := [3]float64{rec[maxDim+2], rec[maxDim+4], rec[maxDim+6]}
	dt := rec[maxDim+7 : maxDim+7+3*maxDim] // column-major: axis i occupies dt[i*maxDim : (i+1)*maxDim]

	if !finite(rec[4*maxDim+7]) || !finite(rec[4*maxDim+8]) || !finite(rec[4*maxDim+9]) || !finite(rec[4*maxDim+10]) {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: non-finite KQMAX1/KQ metadata", ErrCorruptSPK)
	}

	kqmax1 := int32(rec[4*maxDim+7])
	kq := [3]int32{int32(rec[4*maxDim+8]), int32(rec[4*maxDim+9]), int32(rec[4*maxDim+10])}

	if kqmax1 < 2 || kqmax1 > maxAllowedDim+1 {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: KQMAX1=%d, allowed [2,%d]", ErrInvalidOrder, kqmax1, maxAllowedDim+1)
	}

	// KQMAX1/KQ are read straight from the record with no guarantee they
	// stay within this segment's own MAXDIM — unlike the maxAllowedDim+1
	// check above (a fixed library-wide ceiling), this is the check that a
	// small, valid-looking MAXDIM (e.g. 1) can't be paired with a
	// KQMAX1/KQ large enough to index g/dt/w out of their MAXDIM-sized
	// bounds below.
	if kqmax1 > maxDim+1 {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: KQMAX1=%d exceeds this segment's own MAXDIM=%d", ErrInvalidOrder, kqmax1, maxDim)
	}

	for axis, k := range kq {
		if k < 0 || k > maxDim {
			return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: KQ[%d]=%d exceeds MAXDIM=%d", ErrInvalidOrder, axis, k, maxDim)
		}
	}

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
	meta, err := r.ReadDoubles(s.EndAddr-3, s.EndAddr)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	tInit, tLen, rSize := meta[0], meta[1], int32(meta[2])
	if err := validateChebyshevMeta(tInit, tLen, rSize); err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	nCoeffs := (rSize - 2) / 3
	if nCoeffs <= 0 {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: %d coefficients derived from record size %d", ErrRecordTooShort, nCoeffs, rSize)
	}

	idx := max(int32((et-tInit)/tLen), 0)

	recStart := s.StartAddr + idx*rSize

	rec, err := r.ReadDoubles(recStart, recStart+rSize-1)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	if int32(len(rec)) < 2+3*nCoeffs {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: %d doubles (need >= %d)", ErrRecordTooShort, len(rec), 2+3*nCoeffs)
	}

	mid, radius := rec[0], rec[1]
	if !finite(mid) || !finite(radius) || radius == 0 {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: invalid mid=%g/radius=%g", ErrCorruptSPK, mid, radius)
	}

	tau := (et - mid) / radius
	pos.X, vel.X = EvalChebyshev(rec[2:2+nCoeffs], tau, radius, true)
	pos.Y, vel.Y = EvalChebyshev(rec[2+nCoeffs:2+2*nCoeffs], tau, radius, true)
	pos.Z, vel.Z = EvalChebyshev(rec[2+2*nCoeffs:2+3*nCoeffs], tau, radius, true)

	if !finite(pos.X) || !finite(pos.Y) || !finite(pos.Z) || !finite(vel.X) || !finite(vel.Y) || !finite(vel.Z) {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: non-finite evaluated position/velocity", ErrCorruptSPK)
	}

	return pos, vel, nil
}

func evaluateType3(s *Segment, r *Reader, et float64) (pos, vel vector.Vec3, err error) {
	meta, err := r.ReadDoubles(s.EndAddr-3, s.EndAddr)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	tInit, tLen, rSize := meta[0], meta[1], int32(meta[2])
	if err := validateChebyshevMeta(tInit, tLen, rSize); err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	nCoeffs := (rSize - 2) / 6
	if nCoeffs <= 0 {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: %d coefficients derived from record size %d", ErrRecordTooShort, nCoeffs, rSize)
	}

	idx := max(int32((et-tInit)/tLen), 0)

	recStart := s.StartAddr + idx*rSize

	rec, err := r.ReadDoubles(recStart, recStart+rSize-1)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	if int32(len(rec)) < 2+6*nCoeffs {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: %d doubles (need >= %d)", ErrRecordTooShort, len(rec), 2+6*nCoeffs)
	}

	mid, radius := rec[0], rec[1]
	if !finite(mid) || !finite(radius) || radius == 0 {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: invalid mid=%g/radius=%g", ErrCorruptSPK, mid, radius)
	}

	tau := (et - mid) / radius
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
