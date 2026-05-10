package fits

import (
	"encoding/binary"
	"fmt"
	"strconv"
)

// CalcChecksum calculates the 1's complement 32-bit sum over a stream of bytes.
// FITS requires 32-bit words explicitly summing in 1's complement arithmetic natively.
func CalcChecksum(data []byte) uint32 {
	var sum uint64

	// Read in 4-byte chunks directly mapping standard memory sizes
	for i := 0; i+3 < len(data); i += 4 {
		val := binary.BigEndian.Uint32(data[i : i+4])
		sum += uint64(val)
	}

	// Process trailing bytes (should not happen on strict 2880 FITS padding boundaries, but supported)
	remainder := len(data) % 4
	if remainder > 0 {
		var last uint32

		offset := len(data) - remainder
		for i := range remainder {
			last |= uint32(data[offset+i]) << (24 - 8*i)
		}

		sum += uint64(last)
	}

	// Fold the 64-bit accumulated sum down to a 32-bit 1's complement integer
	for (sum >> 32) > 0 {
		sum = (sum & 0xFFFFFFFF) + (sum >> 32)
	}

	return uint32(sum)
}

// ValidateDatasum checks if the DATASUM header matches the computed uint32 sum.
func ValidateDatasum(headerSum string, computed uint32) error {
	expected, err := strconv.ParseUint(headerSum, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid DATASUM header format: %w", err)
	}

	if uint32(expected) != computed {
		return fmt.Errorf("DATASUM mismatch: expected %d, got %d", expected, computed)
	}

	return nil
}

// VerifyChecksum structurally checks if the full encoded block checksum matches
// the absolute HDU FITS string sum (all bits to 1, equivalent to 0xFFFFFFFF).
func VerifyChecksum(sum uint32) bool {
	return sum == 0xFFFFFFFF || sum == 0 // -0 or +0 natively inside 1s complement
}
