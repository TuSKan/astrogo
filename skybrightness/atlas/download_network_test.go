//go:build network
// +build network

package atlas

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/remote"
)

// errTailOffsetTooEarly is returned by tailReaderAt.ReadAt for an offset
// before the fetched tail — meaning the archive's actual layout (central
// directory position) is larger than tailFetchSize anticipated.
var errTailOffsetTooEarly = errors.New("atlas: offset before the fetched tail — archive layout larger than expected")

// requireWorldAtlas skips the test when GFZ Data Services is unreachable —
// per this project's network test policy, a reachability failure must
// never fail CI outright.
func requireWorldAtlas(t *testing.T) {
	t.Helper()

	var d net.Dialer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := d.DialContext(ctx, "tcp", "datapub.gfz.de:443")
	if err != nil {
		t.Skipf("GFZ Data Services unreachable, skipping live test: %v", err)
	}

	_ = conn.Close()
}

// tailReaderAt serves ReadAt out of a fixed in-memory buffer holding only
// the LAST len(buf) bytes of a size-byte remote file — enough for
// archive/zip.NewReader to locate and parse the end-of-central-directory
// record and central directory of this specific (small central directory)
// archive without ever downloading the whole ~653 MB file.
type tailReaderAt struct {
	buf       []byte
	tailStart int64
}

func (t *tailReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < t.tailStart {
		return 0, fmt.Errorf("%w: offset %d, tail starts at %d", errTailOffsetTooEarly, off, t.tailStart)
	}

	start := off - t.tailStart
	if start >= int64(len(t.buf)) {
		return 0, io.EOF
	}

	n := copy(p, t.buf[start:])
	if n < len(p) {
		return n, io.EOF
	}

	return n, nil
}

// tailFetchSize is how much of the archive's tail to range-read — a small
// fraction of the ~653 MB archive, generously covering this file's
// ~260-byte central directory (3 entries) plus its 22-byte EOCD record.
const tailFetchSize = 1 << 20 // 1 MiB

// TestWorldAtlasArchiveUpstreamContract HEADs the real World Atlas archive
// and range-reads only its ZIP central directory (≈1 MB of traffic, not
// the ~653 MB archive) to confirm the upstream file is still the exact
// size and entry name this package's hardcoded constants
// (worldAtlasZipEntry, worldAtlasExtractedSize) assume — this is the whole
// upstream-contract check, catching a silent re-host/re-zip long before a
// user eats a 653 MB failure.
func TestWorldAtlasArchiveUpstreamContract(t *testing.T) {
	requireWorldAtlas(t)

	base, err := remote.URL(remote.WorldAtlas)
	if err != nil {
		t.Fatalf("remote.URL(WorldAtlas): %v", err)
	}

	url := base + worldAtlasZipName

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		t.Fatalf("new HEAD request: %v", err)
	}

	headResp, err := http.DefaultClient.Do(headReq)
	if err != nil {
		t.Fatalf("HEAD %s: %v", url, err)
	}

	_ = headResp.Body.Close()

	if headResp.StatusCode != http.StatusOK {
		t.Fatalf("HEAD %s: status %d", url, headResp.StatusCode)
	}

	size := headResp.ContentLength
	if size <= 0 {
		t.Fatalf("HEAD %s: no usable Content-Length (%d)", url, size)
	}

	tailStart := max(size-tailFetchSize, 0)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", tailStart, size-1))

	rangeResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("range GET %s: %v", url, err)
	}
	defer func() { _ = rangeResp.Body.Close() }()

	if rangeResp.StatusCode != http.StatusPartialContent {
		t.Fatalf("range GET %s: status %d (server may no longer support Range requests)", url, rangeResp.StatusCode)
	}

	tail, err := io.ReadAll(rangeResp.Body)
	if err != nil {
		t.Fatalf("read tail: %v", err)
	}

	zr, err := zip.NewReader(&tailReaderAt{buf: tail, tailStart: tailStart}, size)
	if err != nil {
		t.Fatalf("parse central directory from tail: %v", err)
	}

	var entry *zip.File

	names := make([]string, len(zr.File))
	for i, f := range zr.File {
		names[i] = f.Name

		if f.Name == worldAtlasZipEntry {
			entry = f
		}
	}

	if entry == nil {
		t.Fatalf("entry %q not found in archive; got entries %v", worldAtlasZipEntry, names)
	}

	if entry.UncompressedSize64 != worldAtlasExtractedSize {
		t.Errorf("%s uncompressed size = %d, want %d (worldAtlasExtractedSize) — hardcoded constant is stale, update it",
			worldAtlasZipEntry, entry.UncompressedSize64, worldAtlasExtractedSize)
	}

	const knownApproxSize = 684_266_450

	if size != knownApproxSize {
		t.Logf("archive size changed: now %d bytes (remote.WorldAtlas's ApproxSize documents %d) — update the endpoint if this persists",
			size, knownApproxSize)
	}
}
