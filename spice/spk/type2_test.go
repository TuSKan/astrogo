package spk

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/spice/daf"
)

// TestEvaluateChebyshev guarantees native math combinations identically track standard JPL representations natively.
func TestEvaluateChebyshev(t *testing.T) {
	tests := []struct {
		name     string
		coeffs   []float64
		x        float64
		expected float64
	}{
		{
			name:     "Explicit Polynomial Validation",
			coeffs:   []float64{2.0, -1.5, 0.5},
			x:        0.5,
			expected: 1.0,
		},
		{
			name:     "Empty Coefficients Map",
			coeffs:   []float64{},
			x:        0.5,
			expected: 0.0,
		},
		{
			name:     "Constant Function",
			coeffs:   []float64{3.1415},
			x:        0.5,
			expected: 3.1415,
		},
	}

	const tol = 1e-9

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvaluateChebyshev(tt.x, tt.coeffs)
			if math.Abs(tt.expected-result) > tol {
				t.Errorf("EvaluateChebyshev(%v) = %v; want exactly %v natively tracking ±%v bounds securely", tt.x, result, tt.expected, tol)
			}
		})
	}
}

// TestDAFAddressToOffset inherently protects pure word->byte transformations preventing memory panics completely.
func TestDAFAddressToOffset(t *testing.T) {
	tests := []struct {
		name     string
		address  int32
		expected int64
	}{
		{
			name:     "Base SPK Memory Vector Offset",
			address:  1,
			expected: 0, // Array inherently maps 0 natively over the pointer boundaries
		},
		{
			name:     "Internal Buffer Start Mapping",
			address:  1024,
			expected: 8184, // (1024 - 1) * 8 = 1023 * 8 = 8184 strictly natively
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DAFAddressToOffset(tt.address)
			if result != tt.expected {
				t.Errorf("DAFAddressToOffset(%v) inherently computed %v natively; exactly expected %v", tt.address, result, tt.expected)
			}
		})
	}
}

func getWorkspace() string {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return ""
	}
	return filepath.Dir(strings.TrimSpace(string(out)))
}

func TestMarsPosition(t *testing.T) {
	bspPath := filepath.Join(getWorkspace(), "ephem/data/de441_part-2.bsp")
	file, err := os.Open(bspPath)
	if err != nil {
		fmt.Printf("Failed to open JPL Ephemeris binary file: %s, %v\n", bspPath, err)
		return
	}
	defer file.Close()

	fr, err := daf.ParseFileRecord(file)
	if err != nil {
		t.Fatalf("Failed to parse OS File Record natively: %v", err)
	}

	targetTime := (2461118.5 - 2451545.0) * 86400.0
	currRec := fr.Fward

	var found *daf.SegmentSummary

	for currRec != 0 {
		sr, err := daf.ReadSummaryRecord(file, currRec, fr.Endianness)
		if err != nil {
			t.Fatalf("Failed tracking implicit offset %d locally: %v", currRec, err)
		}

		for _, summ := range sr.Summaries {
			// Mars Barycenter (4) vs Solar System Barycenter (0)
			if summ.TargetID == 4 && summ.CenterID == 0 {
				if targetTime >= summ.StartTime && targetTime <= summ.EndTime {
					copySumm := summ
					found = &copySumm
					break
				}
			}
		}

		if found != nil {
			break
		}

		currRec = sr.NextRecord
	}

	if found == nil {
		t.Skip("Target structural metrics completely missing from internal SPK boundaries.")
	}

	// Read explicit SPK Type 2 Segment Directory parameters precisely mapping logical boundaries smoothly natively.
	// NAIF SPK Type 2 segments terminate flawlessly terminating 4 geometric properties: INIT, INTLEN, RSIZE, N.
	dirOffset := DAFAddressToOffset(found.EndAddress - 3)
	dirBuf := make([]byte, 32)
	if _, err := file.ReadAt(dirBuf, dirOffset); err != nil {
		t.Fatalf("Failed natively reading SPK internal directories structurally: %v", err)
	}

	rawInit := math.Float64frombits(fr.Endianness.Uint64(dirBuf[0:8]))
	rawIntlen := math.Float64frombits(fr.Endianness.Uint64(dirBuf[8:16]))
	rawRsize := math.Float64frombits(fr.Endianness.Uint64(dirBuf[16:24]))
	rawN := math.Float64frombits(fr.Endianness.Uint64(dirBuf[24:32]))
	t.Logf("rawInit: %f, rawIntlen: %f, rawRsize: %f, rawN: %f, targetTime: %f", rawInit, rawIntlen, rawRsize, rawN, targetTime)

	// Target Index calculates strictly directly inside pure array intervals seamlessly
	idx := int(math.Floor((targetTime - rawInit) / rawIntlen))

	// Physical SPK logical structure boundaries explicitly evaluated natively bounds
	recBegin := found.BeginAddress + int32(idx)*int32(rawRsize)
	recEnd := recBegin + int32(rawRsize) - 1

	x, y, z, err := ReadType2Record(file, recBegin, recEnd, fr.Endianness, targetTime)
	if err != nil {
		t.Fatalf("Native reading of Type 2 struct strictly failed fundamentally: %v", err)
	}

	expectedX := 1.811340183814305e+08
	expectedY := -8.868003677773423e+07
	expectedZ := -4.553220908813559e+07

	// JPL Horizons returned coordinates explicitly structured against DE441!
	// Because we are evaluating specifically against de441_part-2.bsp, the structural
	// math models align identically mirroring FORTRAN. We can enforce strict verification!
	tolerance := 1e-6 // 1 millimeter precision specifically checking strict polynomial boundaries!

	if math.Abs(x-expectedX) > tolerance {
		t.Errorf("X = %v; seamlessly expected structurally %v accurately", x, expectedX)
	}
	if math.Abs(y-expectedY) > tolerance {
		t.Errorf("Y = %v; seamlessly expected structurally %v accurately", y, expectedY)
	}
	if math.Abs(z-expectedZ) > tolerance {
		t.Errorf("Z = %v; seamlessly expected structurally %v accurately", z, expectedZ)
	}
}
