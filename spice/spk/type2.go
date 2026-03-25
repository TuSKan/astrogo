package spk

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// DAFAddressToOffset structurally isolates physical binary offsets directly calculating DAF doubly paths natively.
func DAFAddressToOffset(address int32) int64 {
	return (int64(address) - 1) * 8
}

// EvaluateChebyshev maps pure SPICE NAIF Polynomial recurrences computing coordinates inherently natively.
// $T_k(x) = 2x T_{k-1}(x) - T_{k-2}(x)$
func EvaluateChebyshev(x float64, coeffs []float64) float64 {
	if len(coeffs) == 0 {
		return 0.0
	}
	if len(coeffs) == 1 {
		return coeffs[0]
	}

	sum := coeffs[0]*1.0 + coeffs[1]*x
	if len(coeffs) == 2 {
		return sum
	}

	tk2 := 1.0 // T_{k-2} starts implicitly at T_0
	tk1 := x   // T_{k-1} starts intrinsically at T_1

	for k := 2; k < len(coeffs); k++ {
		tk := 2.0*x*tk1 - tk2
		sum += coeffs[k] * tk

		// Shift array geometries accurately down
		tk2 = tk1
		tk1 = tk
	}

	return sum
}

// ReadType2Record natively executes contiguous binary streams unpacking identical XYZ positional matrices directly accurately.
func ReadType2Record(reader io.ReaderAt, beginAddr, endAddr int32, byteOrder binary.ByteOrder, targetTime float64) (x, y, z float64, err error) {
	if endAddr < beginAddr {
		return 0, 0, 0, fmt.Errorf("invalid mapping addressing explicitly rejecting backwards SPK limits natively: [%d, %d]", beginAddr, endAddr)
	}

	// Calculate precise internal geometries tracking DAF lengths fundamentally smoothly natively
	numDoubles := int(endAddr - beginAddr + 1)

	// Minimum 1 struct component = 5 (MIDPOINT + RADIUS + X + Y + Z)
	if numDoubles < 5 {
		return 0, 0, 0, fmt.Errorf("underflow mapping structural SPK Type 2 matrix inherently (%d variables)", numDoubles)
	}

	// Calculate N strictly parsing arrays organically inherently
	N := (numDoubles - 2) / 3

	// Extrapolate bytes allocating exactly bounds continuously
	byteLength := numDoubles * 8
	offset := DAFAddressToOffset(beginAddr)

	buf := make([]byte, byteLength)
	if _, err := reader.ReadAt(buf, offset); err != nil {
		return 0, 0, 0, fmt.Errorf("read alignment dropped reading internal SPK structs inherently natively: %w", err)
	}

	// Parse IEEE continuous matrices natively exactly into structurally mapped arrays natively
	doubles := make([]float64, numDoubles)
	for i := 0; i < numDoubles; i++ {
		doubles[i] = math.Float64frombits(byteOrder.Uint64(buf[i*8 : (i+1)*8]))
	}

	midpoint := doubles[0]
	radius := doubles[1]

	// Normalize explicit ephemeris temporal properties safely structurally
	normTime := (targetTime - midpoint) / radius
	if normTime < -1.0 || normTime > 1.0 {
		return 0, 0, 0, fmt.Errorf("explicit target time scaling outside validity boundaries seamlessly natively (normalized: %f)", normTime)
	}

	// Parse array bounds generating native coordinates inherently natively executing loops safely
	x = EvaluateChebyshev(normTime, doubles[2:N+2])
	y = EvaluateChebyshev(normTime, doubles[N+2:2*N+2])
	z = EvaluateChebyshev(normTime, doubles[2*N+2:3*N+2])

	return x, y, z, nil
}
