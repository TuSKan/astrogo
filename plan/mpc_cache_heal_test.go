package plan

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// mpcErrorPage is what a server answering with an error page under a 200 puts
// where the code list should be.
const mpcErrorPage = "<html>503 Service Unavailable</html>"

// forgetMPCList drops the process's parsed code list, so the next
// MPCObservatories call goes back to remote, and drops it again after t.
func forgetMPCList(t *testing.T) {
	t.Helper()

	forget := func() {
		mpcCache.mu.Lock()
		defer mpcCache.mu.Unlock()

		mpcCache.list = nil
	}

	forget()
	t.Cleanup(forget)
}

// mpcCacheAndSource points remote.MPCObsCodes's cache and source at fresh
// directories and consents to its downloads, for the rest of t. It returns the
// cache with the key the code list caches under, and the source.
func mpcCacheAndSource(t *testing.T) (cache remote.FS, key string, src remote.FS) {
	t.Helper()

	t.Cleanup(remote.Capture(remote.MPCObsCodes).Restore)
	forgetMPCList(t)

	ctx := context.Background()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	srcURL := testutil.FileURL(t, t.TempDir())
	if err := remote.SetURL(remote.MPCObsCodes, srcURL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	src, err := remote.OpenFS(ctx, srcURL)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}

	cache, prefix, err := remote.CacheDir(ctx, remote.MPCObsCodes)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	remote.EnableDownloads(0, remote.MPCObsCodes)

	return cache, prefix + mpcObsCodesFile, src
}

func putMPC(t *testing.T, fsys remote.FS, key, content string) {
	t.Helper()

	if err := remote.WriteFile(context.Background(), fsys, key, bytes.NewReader([]byte(content))); err != nil {
		t.Fatalf("write %s: %v", key, err)
	}
}

// A cached list damaged where it sits, while the source still serves the copy
// it was fetched from, is replaced in the call that finds it (#1014).
func TestMPCObservatoriesReplacesACorruptCachedList(t *testing.T) {
	cache, key, src := mpcCacheAndSource(t)
	putMPC(t, src, mpcObsCodesFile, mpcFixture)

	if _, err := MPCObservatories(context.Background()); err != nil {
		t.Fatalf("first call: %v", err)
	}

	forgetMPCList(t)
	putMPC(t, cache, key, mpcErrorPage)

	list, err := MPCObservatories(context.Background())
	if err != nil {
		t.Fatalf("call over a corrupt cached list: %v", err)
	}

	if want := len(parseFixture(t)); len(list) != want {
		t.Errorf("%d observatories, want the source's %d", len(list), want)
	}
}

// A source serving an error page in place of the list is downloaded once and
// never cached, and the next call is refused without downloading it again.
func TestMPCObservatoriesDownloadsACorruptListOnce(t *testing.T) {
	cache, key, src := mpcCacheAndSource(t)
	putMPC(t, src, mpcObsCodesFile, mpcErrorPage)

	if _, err := MPCObservatories(context.Background()); !errors.Is(err, ErrNoMPCObservatories) {
		t.Fatalf("first call: err %v, want ErrNoMPCObservatories", err)
	}

	if _, err := fs.Stat(cache, key); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the error page was cached (Stat: %v)", err)
	}

	if _, err := MPCObservatories(context.Background()); !errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("second call, source unchanged: err %v, want remote.ErrRejectedSource", err)
	}
}
