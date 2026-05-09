package fits

import (
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestMMapOpenClose(t *testing.T) {
	tmp, err := os.CreateTemp("", "test_mmap_*.fits")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmp.Name())

	headerData := make([]byte, 2880)
	for i := 0; i < 2880; i++ {
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

func TestMMapSeeker(t *testing.T) {
	seeker := &mmapSeeker{
		len: 10,
		off: 0,
	}
	p := make([]byte, 5)

	// Read EOF bypassed
	seeker.off = 10
	n, err := seeker.Read(p)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF, got n=%d, err=%v", n, err)
	}

	// Invalid Seek
	_, err = seeker.Seek(0, 999)
	if err == nil {
		t.Errorf("expected error for invalid whence")
	}

	// Negative Seek
	_, err = seeker.Seek(-10, io.SeekStart)
	if err == nil {
		t.Errorf("expected error for negative target")
	}

	// Valid Seeks
	target, err := seeker.Seek(5, io.SeekStart)
	if err != nil || target != 5 {
		t.Errorf("expected target 5, got %d (err: %v)", target, err)
	}

	target, err = seeker.Seek(2, io.SeekCurrent)
	if err != nil || target != 7 {
		t.Errorf("expected target 7, got %d (err: %v)", target, err)
	}

	target, err = seeker.Seek(-1, io.SeekEnd)
	if err != nil || target != 9 {
		t.Errorf("expected target 9, got %d (err: %v)", target, err)
	}
}
