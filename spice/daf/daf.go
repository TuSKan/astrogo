package daf

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
)

// FileRecord intrinsically tracks pure physical DAF Header variables seamlessly mapped structurally.
type FileRecord struct {
	Endianness binary.ByteOrder
	Fward      int32 // Physical record start mapping the Summary Linked-List intrinsically
}

// ParseFileRecord strictly processes the 1024-byte zero-block mapping intrinsic layouts smoothly natively.
func ParseFileRecord(file io.ReaderAt) (*FileRecord, error) {
	buf := make([]byte, 1024)
	if _, err := file.ReadAt(buf, 0); err != nil {
		return nil, fmt.Errorf("failed structurally processing DAF physical block 0 exactly natively: %w", err)
	}

	endianStr := strings.TrimSpace(string(buf[88:96]))
	var order binary.ByteOrder
	if endianStr == "LTL-IEEE" || endianStr == "LTL-VAX" {
		order = binary.LittleEndian
	} else if strings.HasPrefix(endianStr, "BIG-") {
		order = binary.BigEndian
	} else {
		order = binary.LittleEndian // Safely fallback preventing physical read matrices collapsing mathematically
	}

	fward := int32(order.Uint32(buf[76:80]))

	return &FileRecord{
		Endianness: order,
		Fward:      fward,
	}, nil
}

// SummaryRecord encapsulates exactly the structural mappings spanning NAIF DAF doubly-linked blocks natively.
type SummaryRecord struct {
	NextRecord int32
	PrevRecord int32
	Summaries  []SegmentSummary
}

// SegmentSummary describes explicit properties governing internal Mathematical mappings locally.
type SegmentSummary struct {
	Name         string
	TargetID     int32
	CenterID     int32
	RefFrame     int32
	DataType     int32
	StartTime    float64
	EndTime      float64
	BeginAddress int32
	EndAddress   int32
}

// ReadSummaryRecord maps 1024-byte layout constraints inherently tracking NAIF arrays safely natively.
func ReadSummaryRecord(file io.ReaderAt, recordNumber int32, byteOrder binary.ByteOrder) (*SummaryRecord, error) {
	// A standard NAIF physical record strictly represents exactly 1024 byte blocks explicitly.
	offset := int64(recordNumber-1) * 1024

	buf := make([]byte, 1024)
	if _, err := file.ReadAt(buf, offset); err != nil {
		return nil, fmt.Errorf("failed allocating DAF summary physical representations intrinsically: %w", err)
	}

	// NAIF DAF structure requires NEXT, PREV, and NSUM to inherently occupy the first 24 explicitly float64 bytes
	nextRecord := math.Float64frombits(byteOrder.Uint64(buf[0:8]))
	prevRecord := math.Float64frombits(byteOrder.Uint64(buf[8:16]))
	nSum := math.Float64frombits(byteOrder.Uint64(buf[16:24]))

	numSummaries := int(nSum)

	// In DAF, Name arrays reside directly in the character record bounding exactly sequentially
	nameBuf := make([]byte, 1024)
	if _, err := file.ReadAt(nameBuf, offset+1024); err != nil {
		return nil, fmt.Errorf("failed fetching associated DAF Name Arrays locally: %w", err)
	}

	record := &SummaryRecord{
		NextRecord: int32(nextRecord),
		PrevRecord: int32(prevRecord),
		Summaries:  make([]SegmentSummary, numSummaries),
	}

	for i := 0; i < numSummaries; i++ {
		// Each complete SPK Segment requires exactly 40 bytes explicitly handling ND=2, NI=6
		base := 24 + (i * 40)

		if base+40 > 1024 {
			return nil, fmt.Errorf("invalid DAF configuration isolating structural buffers natively completely")
		}

		startTime := math.Float64frombits(byteOrder.Uint64(buf[base : base+8]))
		endTime := math.Float64frombits(byteOrder.Uint64(buf[base+8 : base+16]))

		targetID := int32(byteOrder.Uint32(buf[base+16 : base+20]))
		centerID := int32(byteOrder.Uint32(buf[base+20 : base+24]))
		refFrame := int32(byteOrder.Uint32(buf[base+24 : base+28]))
		dataType := int32(byteOrder.Uint32(buf[base+28 : base+32]))
		beginAddr := int32(byteOrder.Uint32(buf[base+32 : base+36]))
		endAddr := int32(byteOrder.Uint32(buf[base+36 : base+40]))

		// Standard Name configurations enforce explicit 40 native trailing boundaries directly matching ND/NI scales natively
		nameBase := i * 40
		if nameBase+40 > 1024 {
			return nil, fmt.Errorf("invalid DAF name properties exceeding textual buffer limits naturally")
		}

		nameBytes := nameBuf[nameBase : nameBase+40]
		nameStr := strings.TrimSpace(string(nameBytes)) // Remove pure whitespace offsets formatting smoothly

		record.Summaries[i] = SegmentSummary{
			Name:         nameStr,
			TargetID:     targetID,
			CenterID:     centerID,
			RefFrame:     refFrame,
			DataType:     dataType,
			StartTime:    startTime,
			EndTime:      endTime,
			BeginAddress: beginAddr,
			EndAddress:   endAddr,
		}
	}

	return record, nil
}
