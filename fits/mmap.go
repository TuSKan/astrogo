package fits

import (
	"fmt"

	"golang.org/x/exp/mmap"
)

// MMapFile is a FITS file read from a memory mapping of it. Its images are
// read as [ReadAt] reads them: each one's pixels stay in the mapping, and
// [ImageHDU.Section] copies out only the box asked for, so a section can be
// taken until [MMapFile.Close] unmaps the file and not after.
type MMapFile struct {
	*File

	mapped *mmap.ReaderAt
}

// OpenMmap maps the FITS file at path into memory, read-only, and reads its
// structure from the mapping as [ReadAt] does.
func OpenMmap(path string) (*MMapFile, error) {
	at, err := mmap.Open(path)
	if err != nil {
		return nil, fmt.Errorf("fits: failed to mmap file %s: %w", path, err)
	}

	f, err := ReadAt(at, int64(at.Len()))
	if err != nil {
		if cerr := at.Close(); cerr != nil {
			return nil, fmt.Errorf("%w (and closing the mapping: %w)", err, cerr)
		}

		return nil, err
	}

	return &MMapFile{
		File:   f,
		mapped: at,
	}, nil
}

// Close unmaps the file. A section of one of its images cannot be taken
// afterwards.
func (m *MMapFile) Close() error {
	if m.mapped != nil {
		err := m.mapped.Close()
		if err != nil {
			return fmt.Errorf("fits: close mmap: %w", err)
		}

		return nil
	}

	return nil
}
