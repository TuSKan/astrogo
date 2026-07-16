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

	gofs "github.com/ungerik/go-fs"

	"github.com/TuSKan/astrogo/remote"
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
// explicit remote.EnableDownloads(remote.NAIFSPK, maxSize) call or a
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
	spkFile, err := remote.GetFile(ctx, remote.NAIFSPK, kernel, remote.WithCacheName(kernel))
	if err != nil {
		return nil, fmt.Errorf("jpl: SPK kernel %s: %w", kernel, err)
	}

	ra, err := openReaderAt(spkFile)
	if err != nil {
		return nil, fmt.Errorf("jpl: failed to open SPK: %w", err)
	}

	r, err := NewReader(ra)
	if err != nil {
		closeErr := ra.Close()
		removeErr := spkFile.Remove()

		return nil, errors.Join(err, closeErr, removeErr)
	}

	// Validate physical file size against DAF logical file length
	// FREE is the 1-based index of the first free double precision word.
	// Therefore, (FREE - 1) words * 8 bytes is the absolute minimum byte length.
	size := spkFile.Size()
	expectedMinSize := int64(r.FileRec.FREE-1) * 8

	if size < expectedMinSize {
		closeErr := r.Close()
		removeErr := spkFile.Remove()

		return nil, errors.Join(
			fmt.Errorf("%w: truncated %d bytes, expected min %d bytes", ErrCorruptSPK, size, expectedMinSize),
			closeErr, removeErr,
		)
	}

	// Verify file integrity immediately to auto-heal CI pipelines
	if _, err := r.ReadSummaries(); err != nil {
		closeErr := r.Close()
		removeErr := spkFile.Remove()

		return nil, errors.Join(fmt.Errorf("jpl: corrupt SPK file gracefully deleted: %w", err), closeErr, removeErr)
	}

	// ReadSummaries only parses the DAF directory/summary records, a small
	// fraction of the file — the bulk Chebyshev-coefficient data is never
	// touched by the checks above, so a bit flip there would go undetected.
	// NAIF does not publish per-kernel checksums to verify against, so we
	// record our own SHA-256 the first time a kernel is trusted and compare
	// against it on every later open of the same cached path. Hashing reads
	// through the already-open ra handle instead of opening the file again.
	if err := verifyOrBootstrapChecksum(spkFile, ra, size); err != nil {
		closeErr := r.Close()
		removeErr := spkFile.Remove()
		sumRemoveErr := removeChecksumSidecar(spkFile)

		return nil, errors.Join(fmt.Errorf("jpl: corrupt SPK file gracefully deleted: %w", err), closeErr, removeErr, sumRemoveErr)
	}

	return r, nil
}

// openReaderAt opens f for random access, giving Reader the io.ReaderAt it
// needs for segment lookups. gofs.File.OpenReadSeeker's returned
// ReadSeekCloser already implements io.ReaderAt as part of its interface
// (Read/ReaderAt/Seeker/Closer combined), so it satisfies ReadAtCloser
// directly with no further unwrapping.
func openReaderAt(f gofs.File) (ReadAtCloser, error) {
	rsc, err := f.OpenReadSeeker()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", f, err)
	}

	return rsc, nil
}

// checksumSidecarFile returns the File used to persist a kernel's recorded
// SHA-256, alongside the kernel itself.
func checksumSidecarFile(spkFile gofs.File) gofs.File {
	return spkFile + ".sha256"
}

// removeChecksumSidecar deletes a kernel's checksum sidecar, ignoring a
// missing file (nothing to clean up).
func removeChecksumSidecar(spkFile gofs.File) error {
	sumFile := checksumSidecarFile(spkFile)
	if !sumFile.Exists() {
		return nil
	}

	if err := sumFile.Remove(); err != nil {
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
func verifyOrBootstrapChecksum(spkFile gofs.File, ra io.ReaderAt, size int64) error {
	h := sha256.New()
	if _, err := io.Copy(h, io.NewSectionReader(ra, 0, size)); err != nil {
		return fmt.Errorf("jpl: checksum: read: %w", err)
	}

	sum := hex.EncodeToString(h.Sum(nil))
	sumFile := checksumSidecarFile(spkFile)

	if !sumFile.Exists() {
		if err := remote.Save(strings.NewReader(sum), sumFile); err != nil {
			return fmt.Errorf("jpl: checksum: write sidecar: %w", err)
		}

		return nil
	}

	existing, err := sumFile.ReadAll()
	if err != nil {
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

// ReadSummaries reads all segments summaries.
func (r *Reader) ReadSummaries() ([]Summary, error) {
	var summaries []Summary

	next := r.FileRec.FWD

	for next != 0 {
		buf := make([]byte, RecordSize)
		if _, err := r.F.ReadAt(buf, int64(next-1)*RecordSize); err != nil {
			return nil, fmt.Errorf("spk: read summary record: %w", err)
		}

		fwdFloat := math.Float64frombits(r.FileRec.Order.Uint64(buf[0:8]))
		fwd := int32(fwdFloat)
		nSum := int32(math.Float64frombits(r.FileRec.Order.Uint64(buf[16:24])))
		sumLen := int(r.FileRec.ND+(r.FileRec.NI+1)/2) * 8

		for i := range nSum {
			offset := 24 + int(i)*sumLen
			sumBuf := buf[offset : offset+sumLen]

			s := Summary{
				Doubles:  make([]float64, r.FileRec.ND),
				Integers: make([]int32, r.FileRec.NI),
			}

			for d := range r.FileRec.ND {
				bits := r.FileRec.Order.Uint64(sumBuf[d*8 : (d+1)*8])
				s.Doubles[d] = math.Float64frombits(bits)
			}

			intStart := int(r.FileRec.ND) * 8
			for j := range r.FileRec.NI {
				s.Integers[j] = int32(r.FileRec.Order.Uint32(sumBuf[intStart+int(j)*4 : intStart+int(j+1)*4]))
			}

			summaries = append(summaries, s)
		}

		next = fwd
	}

	return summaries, nil
}

// ReadDoubles reads a range of float64 from the data area.
func (r *Reader) ReadDoubles(startWord, endWord int32) ([]float64, error) {
	count := endWord - startWord + 1
	if count <= 0 {
		return nil, fmt.Errorf("%w: %d to %d", ErrInvalidWordBounds, startWord, endWord)
	}

	buf := make([]byte, count*8)

	n, err := r.F.ReadAt(buf, int64(startWord-1)*8)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("spk: read doubles: %w", err)
	}

	if n < len(buf) && (errors.Is(err, io.EOF) || err == nil) {
		return nil, fmt.Errorf("%w: unexpected EOF reading word %d", ErrCorruptSPK, startWord)
	}

	res := make([]float64, count)
	for i := range count {
		bits := r.FileRec.Order.Uint64(buf[i*8 : (i+1)*8])
		res[i] = math.Float64frombits(bits)
	}

	return res, nil
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

func evaluateType21(s *Segment, r *Reader, et float64) (pos, vel vector.Vec3, err error) {
	// Type 21 segments have N records, then N epochs, then N.
	meta, err := r.ReadDoubles(s.EndAddr, s.EndAddr)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	nRecs := int32(meta[0])
	if nRecs <= 0 {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: %d", ErrInvalidRecordCount, nRecs)
	}

	epochs, err := r.ReadDoubles(s.EndAddr-nRecs, s.EndAddr-1)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	// Binary search for the epoch interval containing et.
	// sort.Search returns the smallest i such that epochs[i] > et,
	// so the record index is (i - 1), clamped to valid bounds.
	idx := int32(sort.Search(int(nRecs), func(i int) bool {
		return epochs[i] > et
	}))
	if idx > 0 {
		idx--
	}

	if idx >= nRecs {
		idx = nRecs - 1
	}

	// Calculate record length
	recordAreaLen := s.EndAddr - nRecs - s.StartAddr
	L := recordAreaLen / nRecs

	recStart := s.StartAddr + idx*L

	rec, err := r.ReadDoubles(recStart, recStart+L-1)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	// Validate record length: need at least 68 doubles (t0 + 15 dt + 3 p0 + 3 v0 + 45 MDA + maxOrd).
	if len(rec) < 68 {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: %d doubles (need >= 68)", ErrRecordTooShort, len(rec))
	}

	// Extended Modified Difference Array Algorithm
	t0 := rec[0]
	dt := rec[1:16]
	p0 := rec[16:19]
	v0 := rec[19:22]
	mda := rec[22 : 22+45]
	maxOrd := int(rec[67])

	// Bounds-check maxOrd against the fixed-size arrays.
	const maxAllowedOrd = 15
	if maxOrd < 0 || maxOrd > maxAllowedOrd {
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("%w: maxOrd=%d, allowed [0,%d]", ErrInvalidOrder, maxOrd, maxAllowedOrd)
	}

	// rec[68:71] are additional weights W if needed, but we calculate them

	delta := et - t0
	if delta == 0 {
		return vector.Vec3{X: p0[0], Y: p0[1], Z: p0[2]}, vector.Vec3{X: v0[0], Y: v0[1], Z: v0[2]}, nil
	}

	// Precompute recursive weights
	var (
		g  [maxAllowedOrd + 1]float64
		gd [maxAllowedOrd + 1]float64
		w  [maxAllowedOrd]float64
	)

	tp := delta
	g[0] = 1.0
	gd[0] = 0.0

	for i := 1; i <= maxOrd; i++ {
		w[i-1] = tp / dt[i-1]
		tp = delta + dt[i-1]
		g[i] = w[i-1] * g[i-1]
		gd[i] = w[i-1]*gd[i-1] + g[i-1]/dt[i-1]
	}

	// Interpolate each component
	posArr := [3]float64{p0[0], p0[1], p0[2]}
	velArr := [3]float64{v0[0], v0[1], v0[2]}

	for i := 1; i <= maxOrd; i++ {
		for j := range 3 {
			// MDA is 3x15, stored as [order1_x, order1_y, order1_z, order2_x, ...]
			val := mda[(i-1)*3+j]
			posArr[j] += g[i] * val
			velArr[j] += gd[i] * val
		}
	}

	return vector.Vec3{X: posArr[0], Y: posArr[1], Z: posArr[2]},
		vector.Vec3{X: velArr[0], Y: velArr[1], Z: velArr[2]}, nil
}

func evaluateType2(s *Segment, r *Reader, et float64) (pos, vel vector.Vec3, err error) {
	meta, err := r.ReadDoubles(s.EndAddr-3, s.EndAddr)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	tInit, tLen, rSize := meta[0], meta[1], int32(meta[2])
	nCoeffs := (rSize - 2) / 3

	idx := max(int32((et-tInit)/tLen), 0)

	recStart := s.StartAddr + idx*rSize

	rec, err := r.ReadDoubles(recStart, recStart+rSize-1)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	mid, radius := rec[0], rec[1]
	tau := (et - mid) / radius
	pos.X, vel.X = EvalChebyshev(rec[2:2+nCoeffs], tau, radius, true)
	pos.Y, vel.Y = EvalChebyshev(rec[2+nCoeffs:2+2*nCoeffs], tau, radius, true)
	pos.Z, vel.Z = EvalChebyshev(rec[2+2*nCoeffs:2+3*nCoeffs], tau, radius, true)

	return pos, vel, nil
}

func evaluateType3(s *Segment, r *Reader, et float64) (pos, vel vector.Vec3, err error) {
	meta, err := r.ReadDoubles(s.EndAddr-3, s.EndAddr)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	tInit, tLen, rSize := meta[0], meta[1], int32(meta[2])
	nCoeffs := (rSize - 2) / 6

	idx := max(int32((et-tInit)/tLen), 0)

	recStart := s.StartAddr + idx*rSize

	rec, err := r.ReadDoubles(recStart, recStart+rSize-1)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	mid, radius := rec[0], rec[1]
	tau := (et - mid) / radius
	pos.X, _ = EvalChebyshev(rec[2:2+nCoeffs], tau, radius, false)
	pos.Y, _ = EvalChebyshev(rec[2+nCoeffs:2+2*nCoeffs], tau, radius, false)
	pos.Z, _ = EvalChebyshev(rec[2+2*nCoeffs:2+3*nCoeffs], tau, radius, false)
	vStart := 2 + 3*nCoeffs
	vel.X, _ = EvalChebyshev(rec[vStart:vStart+nCoeffs], tau, radius, false)
	vel.Y, _ = EvalChebyshev(rec[vStart+nCoeffs:vStart+2*nCoeffs], tau, radius, false)
	vel.Z, _ = EvalChebyshev(rec[vStart+2*nCoeffs:vStart+3*nCoeffs], tau, radius, false)

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
