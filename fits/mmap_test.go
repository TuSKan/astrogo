package fits

import (
	"fmt"
	"os"
	"testing"
)

func TestMMapOpenClose(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "test_mmap_*.fits")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	headerData := make([]byte, 2880)
	for i := range 2880 {
		headerData[i] = ' '
	}

	copy(headerData, []byte(fmt.Sprintf("%-80s", "SIMPLE  =                    T / ")))
	copy(headerData[80:], []byte(fmt.Sprintf("%-80s", "BITPIX  =                    8 / ")))
	copy(headerData[160:], []byte(fmt.Sprintf("%-80s", "NAXIS   =                    0 / ")))
	copy(headerData[240:], []byte(fmt.Sprintf("%-80s", "END")))

	if _, err := tmp.Write(headerData); err != nil {
		t.Fatalf("failed to write header: %v", err)
	}

	if err := tmp.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	mapped, err := OpenMmap(tmp.Name())
	if err != nil {
		t.Fatalf("failed to open mmap: %v", err)
	}

	if len(mapped.HDUs) != 1 {
		t.Errorf("expected 1 hdu, got %d", len(mapped.HDUs))
	}

	err = mapped.Close()
	if err != nil {
		t.Errorf("failed to close mmap: %v", err)
	}

	// Test nonexistent
	_, err = OpenMmap("does_not_exist_at_all_hopefully.fits")
	if err == nil {
		t.Errorf("expected error for nonexistent file")
	}
}
