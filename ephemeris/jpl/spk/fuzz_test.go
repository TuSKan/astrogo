package spk_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
)

// This file fuzzes the SPK/DAF binary parser against corrupted, truncated,
// and adversarial input — none of it goes through remote's consent gate or
// touches the network; every seed here is a hand-built byte literal (no
// checked-in binary fixture) or the reserialized word array from
// TestEvaluateType21 in reader_test.go. Any crasher Go writes to
// testdata/fuzz/ on a failing run should be committed as a regression test
// for the bug it found — see CLAUDE.md's "Fuzzing" note for the manual,
// longer-running invocation (`-fuzz=... -fuzztime=60s`); the seed corpus
// below runs as a plain test under `go test ./...` with no flags needed.
//
// Property under test throughout: parsing/evaluating attacker-influenceable
// bytes (a downloaded, truncated, or corrupted kernel) must never panic or
// hang — it must return an error. Correctness of well-formed data is
// already covered by TestSPKReader/TestEvaluateType21 elsewhere in this
// package.

// wordsToBytes packs vals as consecutive little-endian float64 DAF words
// (1-based addressing, matching Reader.ReadDoubles), the same layout
// newWordBuffer (reader_test.go) uses for a *spk.Reader — this returns the
// raw bytes instead, for use as a fuzz corpus seed.
func wordsToBytes(vals []float64) []byte {
	buf := make([]byte, len(vals)*8)
	for i, v := range vals {
		binary.LittleEndian.PutUint64(buf[i*8:], math.Float64bits(v))
	}

	return buf
}

// buildDAFHeader constructs a syntactically valid 1024-byte DAF file
// record: the format identifier at [88:96] plus the ND/NI/FWD fields
// NewReader decodes. BWD/FREE (unused by any parsing in this package) and
// the ID word text are left zeroed — NewReader doesn't validate any of them.
func buildDAFHeader(nd, ni, fwd int32, bigEndian bool) []byte {
	buf := make([]byte, spk.RecordSize)

	order, format := binary.ByteOrder(binary.LittleEndian), "LTL-IEEE"
	if bigEndian {
		order, format = binary.BigEndian, "BIG-IEEE"
	}

	copy(buf[88:96], format)
	order.PutUint32(buf[8:12], uint32(nd))
	order.PutUint32(buf[12:16], uint32(ni))
	order.PutUint32(buf[76:80], uint32(fwd))

	return buf
}

// buildSummaryRecordSPK constructs a 1024-byte DAF summary/directory record
// holding exactly one ND=2/NI=6 (the standard SPK shape) summary — the
// layout ReadSummaries decodes: the next-record pointer and summary count
// as float64 words at [0:8]/[16:24], then each summary's doubles followed
// by its integers starting at byte offset 24.
func buildSummaryRecordSPK(order binary.ByteOrder, next int32, startET, endET float64, target, center, frame, segType, startAddr, endAddr int32) []byte {
	buf := make([]byte, spk.RecordSize)

	order.PutUint64(buf[0:8], math.Float64bits(float64(next)))
	order.PutUint64(buf[16:24], math.Float64bits(1)) // nSum = 1

	order.PutUint64(buf[24:32], math.Float64bits(startET))
	order.PutUint64(buf[32:40], math.Float64bits(endET))

	order.PutUint32(buf[40:44], uint32(target))
	order.PutUint32(buf[44:48], uint32(center))
	order.PutUint32(buf[48:52], uint32(frame))
	order.PutUint32(buf[52:56], uint32(segType))
	order.PutUint32(buf[56:60], uint32(startAddr))
	order.PutUint32(buf[60:64], uint32(endAddr))

	return buf
}

func FuzzNewReaderReadSummaries(f *testing.F) {
	f.Add(buildDAFHeader(2, 6, 0, false)) // valid header, no summary records

	validWithOneSummary := append(
		buildDAFHeader(2, 6, 2, false),
		buildSummaryRecordSPK(binary.LittleEndian, 0, -1e8, 1e8, 3, 10, 1, 2, 100, 200)...)
	f.Add(validWithOneSummary)

	f.Add(buildDAFHeader(2, 6, 0, true)) // big-endian variant

	f.Add(make([]byte, spk.RecordSize)) // all-zero header: ND=NI=0, FWD=0
	f.Add(make([]byte, 100))            // truncated: shorter than one record

	f.Add(buildDAFHeader(1<<20, 1<<20, 0, false)) // extreme ND/NI
	f.Add(buildDAFHeader(2, 6, -5, false))        // negative FWD

	cyclic := append(
		buildDAFHeader(2, 6, 2, false),
		buildSummaryRecordSPK(binary.LittleEndian, 2, 0, 0, 0, 0, 0, 0, 0, 0)...) // record 2's FWD points back to itself
	f.Add(cyclic)

	f.Fuzz(func(_ *testing.T, data []byte) {
		r, err := spk.NewReader(fakeReaderAt{bytes.NewReader(data)})
		if err != nil {
			return // a rejected malformed header is a correct, expected outcome
		}

		_, _ = r.ReadSummaries() // property: never panics or hangs, error is fine
	})
}

func FuzzEvaluateSegment(f *testing.F) {
	// Type 21: TestEvaluateType21's synthetic single-record segment,
	// reserialized to raw bytes (see reader_test.go for the field layout
	// this encodes).
	const (
		t0        = 100.0
		maxDim21  = 1.0
		kqmax1    = 2.0
		startAddr = 1
	)

	refPos := [3]float64{10, 20, 30}
	refVel := [3]float64{1, 2, 3}
	type21Rec := []float64{
		t0, 1.0,
		refPos[0], refVel[0], refPos[1], refVel[1], refPos[2], refVel[2],
		99, 99, 99,
		kqmax1,
		0, 0, 0,
	}
	type21Words := append(append([]float64{}, type21Rec...), 999999.0, maxDim21, 1.0)

	f.Add(uint8(2), int32(startAddr), int32(len(type21Words)), t0, wordsToBytes(type21Words))

	// Type 2: a single Chebyshev record, one coefficient per axis
	// (rSize=5), evaluated exactly at its own tInit so idx=0 with no
	// boundary edge cases.
	type2Words := []float64{
		0.0, 1.0, 7.0, 8.0, 9.0, // rec: mid, radius, cx, cy, cz
		0.0, 1.0, 5.0, 0.0, // meta at EndAddr-3..EndAddr: tInit, tLen, rSize, (unused)
	}
	f.Add(uint8(0), int32(1), int32(len(type2Words)), 0.0, wordsToBytes(type2Words))

	// A few structurally-plausible-but-wrong mutations: truncated data,
	// all-zero data, and an unsupported segment type (exercises the
	// EvaluateSegment switch's default branch).
	f.Add(uint8(1), int32(1), int32(9), 0.0, wordsToBytes(type2Words)[:10])
	f.Add(uint8(2), int32(1), int32(20), 50.0, make([]byte, 200))
	f.Add(uint8(0), int32(0), int32(0), math.NaN(), []byte{})

	f.Fuzz(func(_ *testing.T, typeSel uint8, startAddr, endAddr int32, et float64, data []byte) {
		segType := [3]int32{2, 3, 21}[int(typeSel)%3]

		r := &spk.Reader{
			F:       fakeReaderAt{bytes.NewReader(data)},
			FileRec: spk.FileRecord{Order: binary.LittleEndian},
		}
		seg := &spk.Segment{
			Type:      segType,
			StartAddr: startAddr,
			EndAddr:   endAddr,
			StartET:   -1e12,
			EndET:     1e12,
		}

		_, _, _ = spk.EvaluateSegment(seg, r, et) // property: never panics or hangs
	})
}

func FuzzReadDoubles(f *testing.F) {
	seed := wordsToBytes([]float64{1, 2, 3, 4, 5})
	f.Add(int32(1), int32(5), seed)
	f.Add(int32(1), int32(1), seed)
	f.Add(int32(5), int32(1), seed) // endWord < startWord: count <= 0
	f.Add(int32(-1), int32(3), seed)
	f.Add(int32(1), int32(1<<30), seed) // extreme count
	f.Add(int32(0), int32(0), []byte{})

	f.Fuzz(func(_ *testing.T, startWord, endWord int32, data []byte) {
		r := &spk.Reader{
			F:       fakeReaderAt{bytes.NewReader(data)},
			FileRec: spk.FileRecord{Order: binary.LittleEndian},
		}

		_, _ = r.ReadDoubles(startWord, endWord) // property: never panics or hangs
	})
}
