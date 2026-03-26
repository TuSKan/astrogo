package jpl

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"

	"github.com/TuSKan/astrogo/vector"
)

// Download fetches a file from a URL and saves it to the target path.
func Download(url, path string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jpl: download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

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

// Reader provides tools to read DAF/SPK files.
type Reader struct {
	F       *os.File
	FileRec FileRecord
}

// Open opens a DAF/SPK file and reads its metadata.
func Open(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, RecordSize)
	if _, err := f.ReadAt(buf, 0); err != nil {
		f.Close()
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
	buf := make([]byte, count*8)
	if _, err := r.F.ReadAt(buf, int64(startWord-1)*8); err != nil && err != io.EOF {
		return nil, err
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
	default:
		return vector.Vec3{}, vector.Vec3{}, fmt.Errorf("jpl: unsupported SPK segment type %d", s.Type)
	}
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
