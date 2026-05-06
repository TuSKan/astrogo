package spk

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/TuSKan/astrogo/internal/tools"
	"github.com/TuSKan/astrogo/vector"
)

const JPL_SPK_KERNEL_URI = "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/spk/"

// RecordSize is the standard DAF record size in bytes.
const RecordSize = 1024

// FileRecord represents the DAF file record.
type FileRecord struct {
	IDWord         uint64
	ND, NI         int32
	FWD, BWD, FREE int32
	Order          binary.ByteOrder
}

// Summary represents a DAF segment summary.
type Summary struct {
	Doubles  []float64
	Integers []int32
}

type ReadAtCloser interface {
	io.ReaderAt
	io.Closer
}

// Reader provides tools to read DAF/SPK files.
type Reader struct {
	F       ReadAtCloser
	FileRec FileRecord
}

func CacheDownload(kernel, path string) (*Reader, error) {
	spkPath := filepath.Join(path, kernel)

	if err := os.MkdirAll(filepath.Dir(spkPath), 0755); err != nil {
		return nil, fmt.Errorf("jpl: failed to create parent dir for SPK %s: %w", spkPath, err)
	}

	if _, err := os.Stat(spkPath); os.IsNotExist(err) {
		spkURI := JPL_SPK_KERNEL_URI + kernel
		fmt.Printf("jpl: downloading %s...\n", spkURI)
		if err := tools.Download(spkURI, spkPath); err != nil {
			return nil, fmt.Errorf("jpl: failed to download SPK: %w", err)
		}
	}

	file, err := os.Open(spkPath)
	if err != nil {
		return nil, fmt.Errorf("jpl: failed to open SPK: %w", err)
	}

	r, err := NewReader(file)
	if err != nil {
		file.Close()
		os.Remove(spkPath)
		return nil, err
	}

	// Validate physical file size against DAF logical file length
	// FREE is the 1-based index of the first free double precision word.
	// Therefore, (FREE - 1) words * 8 bytes is the absolute minimum byte length.
	if stat, err := file.Stat(); err == nil {
		expectedMinSize := int64(r.FileRec.FREE-1) * 8
		if stat.Size() < expectedMinSize {
			r.Close()
			os.Remove(spkPath)
			return nil, fmt.Errorf("jpl: corrupt SPK file gracefully deleted (truncated: %d bytes, expected min %d bytes)", stat.Size(), expectedMinSize)
		}
	}

	// Verify file integrity immediately to auto-heal CI pipelines
	if _, err := r.ReadSummaries(); err != nil {
		r.Close()
		os.Remove(spkPath)
		return nil, fmt.Errorf("jpl: corrupt SPK file gracefully deleted: %w", err)
	}

	return r, nil
}

// New opens a DAF/SPK file and reads its metadata.
func NewReader(f ReadAtCloser) (*Reader, error) {
	buf := make([]byte, RecordSize)
	if _, err := f.ReadAt(buf, 0); err != nil {
		return nil, err
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
	return r.F.Close()
}

// ReadSummaries reads all segments summaries.
func (r *Reader) ReadSummaries() ([]Summary, error) {
	var summaries []Summary
	next := r.FileRec.FWD

	for next != 0 {
		buf := make([]byte, RecordSize)
		if _, err := r.F.ReadAt(buf, int64(next-1)*RecordSize); err != nil {
			return nil, err
		}

		fwdFloat := math.Float64frombits(r.FileRec.Order.Uint64(buf[0:8]))
		fwd := int32(fwdFloat)
		nSum := int32(math.Float64frombits(r.FileRec.Order.Uint64(buf[16:24])))
		sumLen := int(r.FileRec.ND+(r.FileRec.NI+1)/2) * 8

		for i := 0; i < int(nSum); i++ {
			offset := 24 + i*sumLen
			sumBuf := buf[offset : offset+sumLen]

			s := Summary{
				Doubles:  make([]float64, r.FileRec.ND),
				Integers: make([]int32, r.FileRec.NI),
			}

			for d := 0; d < int(r.FileRec.ND); d++ {
				bits := r.FileRec.Order.Uint64(sumBuf[d*8 : (d+1)*8])
				s.Doubles[d] = math.Float64frombits(bits)
			}

			intStart := int(r.FileRec.ND) * 8
			for j := 0; j < int(r.FileRec.NI); j++ {
				s.Integers[j] = int32(r.FileRec.Order.Uint32(sumBuf[intStart+j*4 : intStart+(j+1)*4]))
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
		return nil, fmt.Errorf("jpl/spk: invalid double precision word bounds (%d to %d)", startWord, endWord)
	}

	buf := make([]byte, count*8)
	n, err := r.F.ReadAt(buf, int64(startWord-1)*8)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n < len(buf) && (err == io.EOF || err == nil) {
		return nil, fmt.Errorf("jpl/spk: corrupt file (unexpected EOF reading word %d)", startWord)
	}

	res := make([]float64, count)
	for i := 0; i < int(count); i++ {
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
	for i := len(segments) - 1; i >= 0; i-- {
		s := &segments[i]
		if s.Target == targetID && et >= s.StartET && et <= s.EndET {
			return s, nil
		}
	}
	return nil, fmt.Errorf("jpl: no coverage for target %d at ET %f", targetID, et)
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
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("jpl: unsupported SPK segment type %d", s.Type)
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
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("jpl: type 21 segment has invalid record count: %d", nRecs)
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
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("jpl: type 21 record too short: %d doubles (need >= 68)", len(rec))
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
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("jpl: type 21 maxOrd=%d out of valid range [0,%d]", maxOrd, maxAllowedOrd)
	}

	// rec[68:71] are additional weights W if needed, but we calculate them

	delta := et - t0
	if delta == 0 {
		return vector.Vec3{X: p0[0], Y: p0[1], Z: p0[2]}, vector.Vec3{X: v0[0], Y: v0[1], Z: v0[2]}, nil
	}

	// Precompute recursive weights
	var g [maxAllowedOrd + 1]float64
	var gd [maxAllowedOrd + 1]float64
	var w [maxAllowedOrd]float64

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
		for j := 0; j < 3; j++ {
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
	idx := int32((et - tInit) / tLen)
	if idx < 0 {
		idx = 0
	}
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
	idx := int32((et - tInit) / tLen)
	if idx < 0 {
		idx = 0
	}
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
