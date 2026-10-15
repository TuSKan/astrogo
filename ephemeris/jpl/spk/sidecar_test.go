package spk

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// The checksum sidecar under another process's hand (#784).
//
// go test ./... runs a binary per package against one shared cache, and
// several open de440s.bsp. On Windows a sidecar being renamed into place by
// one cannot be opened by another — the rename holds delete access that Go's
// open does not share — and a rename over a sidecar another has open fails
// with "Access is denied". Either failed the open of a kernel that was fine.

// errSharingViolation is what Windows reports for the first of those.
var errSharingViolation = errors.New("the process cannot access the file because it is being used by another process")

// errAccessDenied is what it reports for the second.
var errAccessDenied = errors.New("access is denied")

// errCannotCreate means the cache under the test cannot be written at all.
var errCannotCreate = errors.New("the test's cache cannot create")

// contendedCache is a cache another process is using: opening a checksum
// sidecar fails with a sharing violation the first lockedOpens times, and
// create, when set, stands in for this process's sidecar write.
type contendedCache struct {
	remote.FS

	lockedOpens int
	opens       int
	create      func(name string) (io.WriteCloser, error)
}

func (c *contendedCache) Open(name string) (fs.File, error) {
	if strings.HasSuffix(name, ".sha256") {
		c.opens++

		if c.opens <= c.lockedOpens {
			return nil, &fs.PathError{Op: "open", Path: name, Err: errSharingViolation}
		}
	}

	return c.FS.Open(name) //nolint:wrapcheck // a stand-in for the filesystem, passing its errors through
}

// Create is what remote.WriteFile asks for. Without it the embedded FS's
// Create is hidden and every write would report the cache read-only.
func (c *contendedCache) Create(name string) (io.WriteCloser, error) {
	if c.create != nil {
		return c.create(name)
	}

	creator, ok := c.FS.(interface {
		Create(name string) (io.WriteCloser, error)
	})
	if !ok {
		return nil, fmt.Errorf("%w: %s", errCannotCreate, name)
	}

	return creator.Create(name) //nolint:wrapcheck // a stand-in for the filesystem, passing its errors through
}

func emptyCache(t *testing.T) remote.FS {
	t.Helper()

	fsys, err := remote.OpenFS(t.Context(), testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("open cache: %v", err)
	}

	return fsys
}

const sidecarKernel = "jpl/planets/de440s.bsp"

var sidecarKernelBytes = strings.NewReader("kernel bytes")

func sidecarSum(content string) string {
	h := sha256.Sum256([]byte(content))

	return hex.EncodeToString(h[:])
}

func TestChecksumSidecarIsReadThroughABriefLock(t *testing.T) {
	inner := emptyCache(t)

	if err := remote.WriteFile(t.Context(), inner, checksumSidecarKey(sidecarKernel), strings.NewReader(sidecarSum("kernel bytes"))); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}

	// Locked for two opens, as a rename a few milliseconds long would hold
	// it, and readable on the third.
	cache := &contendedCache{FS: inner, lockedOpens: 2}

	if err := verifyOrBootstrapChecksum(t.Context(), cache, sidecarKernel, sidecarKernelBytes, sidecarKernelBytes.Size()); err != nil {
		t.Fatalf("a sidecar locked for two opens failed the kernel: %v", err)
	}

	if cache.opens != 3 {
		t.Errorf("the sidecar was opened %d times, want 3", cache.opens)
	}
}

// A lock that outlasts the retries is still reported, and as a failure to
// read rather than as a corrupt kernel, so the kernel is kept.
func TestChecksumSidecarLockedThroughoutIsAReadFailure(t *testing.T) {
	inner := emptyCache(t)

	if err := remote.WriteFile(t.Context(), inner, checksumSidecarKey(sidecarKernel), strings.NewReader(sidecarSum("kernel bytes"))); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}

	cache := &contendedCache{FS: inner, lockedOpens: 1000}

	err := verifyOrBootstrapChecksum(t.Context(), cache, sidecarKernel, sidecarKernelBytes, sidecarKernelBytes.Size())
	if !errors.Is(err, errSharingViolation) || errors.Is(err, ErrCorruptSPK) {
		t.Fatalf("verify = %v, want the sharing violation and not ErrCorruptSPK", err)
	}
}

// When this process's bootstrap write is refused because another process has
// just recorded the sidecar, the other's record is checked in its place.
func TestChecksumBootstrapTakesAnotherProcesssRecord(t *testing.T) {
	tests := []struct {
		name      string
		recorded  string // what the other process wrote, or "" for nothing
		wantError error
	}{
		{name: "the same hash", recorded: sidecarSum("kernel bytes")},
		{name: "a different hash", recorded: sidecarSum("other bytes"), wantError: ErrCorruptSPK},
		{name: "nothing", wantError: errAccessDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := emptyCache(t)

			cache := &contendedCache{FS: inner}
			cache.create = func(name string) (io.WriteCloser, error) {
				if tt.recorded != "" {
					if err := remote.WriteFile(t.Context(), inner, name, strings.NewReader(tt.recorded)); err != nil {
						t.Fatalf("the other process's write: %v", err)
					}
				}

				return nil, errAccessDenied
			}

			err := verifyOrBootstrapChecksum(t.Context(), cache, sidecarKernel, sidecarKernelBytes, sidecarKernelBytes.Size())

			if tt.wantError == nil && err != nil {
				t.Fatalf("verify = %v, want nil: the record is this kernel's own hash", err)
			}

			if tt.wantError != nil && !errors.Is(err, tt.wantError) {
				t.Fatalf("verify = %v, want %v", err, tt.wantError)
			}
		})
	}
}
