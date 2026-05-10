package fits

import (
	"errors"
	"fmt"
	"io"

	"golang.org/x/exp/mmap"
)

// MMapFile represents a memory-mapped FITS dataset.
type MMapFile struct {
	*File

	mapped *mmap.ReaderAt
}

// OpenMmap creates a zero-copy memory mapped instance of a FITS file.
// This is strictly read-only and safely bypasses IO overhead for enormous datasets.
func OpenMmap(path string) (*MMapFile, error) {
	at, err := mmap.Open(path)
	if err != nil {
		return nil, fmt.Errorf("fits: failed to mmap file %s: %w", path, err)
	}

	// mmap.ReaderAt only exposes ReadAt and Len. Our core parser expects io.Reader + io.Seeker.
	wrap := &mmapSeeker{
		at:  at,
		off: 0,
		len: int64(at.Len()),
	}

	f, err := Read(wrap)
	if err != nil {
		return nil, err
	}

	return &MMapFile{
		File:   f,
		mapped: at,
	}, nil
}

// Close gracefully releases the memory mapped handle mapping.
func (m *MMapFile) Close() error {
	if m.mapped != nil {
		return m.mapped.Close()
	}

	return nil
}

// mmapSeeker implements io.ReadSeeker bridging an mmap.ReaderAt
type mmapSeeker struct {
	at  *mmap.ReaderAt
	off int64
	len int64
}

func (m *mmapSeeker) Read(p []byte) (int, error) {
	if m.off >= m.len {
		return 0, io.EOF
	}

	n, err := m.at.ReadAt(p, m.off)
	m.off += int64(n)

	return n, err
}

func (m *mmapSeeker) Seek(offset int64, whence int) (int64, error) {
	var target int64

	switch whence {
	case io.SeekStart:
		target = offset
	case io.SeekCurrent:
		target = m.off + offset
	case io.SeekEnd:
		target = m.len + offset
	default:
		return 0, fmt.Errorf("mmapSeeker: invalid whence %d", whence)
	}

	if target < 0 {
		return 0, errors.New("mmapSeeker: negative offset")
	}

	m.off = target

	return target, nil
}
