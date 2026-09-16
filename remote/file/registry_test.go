package file_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote/file"
)

// The scheme registry and the context seam, tested directly.
//
// Both are reached through remote in ordinary use, which means a failure here
// would otherwise arrive as "the fetch went wrong" with the endpoint registry,
// the consent gate and a live source between the assertion and the defect.
// These take a URL and nothing else, so they can be asked what they do.

// seedDir writes a small tree and returns its file:// URL.
func seedDir(t *testing.T, files map[string]string) (dir, fsURL string) {
	t.Helper()

	dir = t.TempDir()

	for name, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))

		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return dir, testutil.FileURL(t, dir)
}

// TestSchemesListsWhatIsRegistered covers the answer to the question an
// unregistered scheme raises.
//
// It is exported because the failure it explains — a missing blank import — is
// invisible otherwise: the URL is well-formed, the endpoint is real, and the
// only symptom is that nothing can open it.
func TestSchemesListsWhatIsRegistered(t *testing.T) {
	t.Parallel()

	got := file.Schemes()

	if len(got) == 0 {
		t.Fatal("Schemes() is empty; the compiled-in backends did not register")
	}

	for _, want := range []string{"file"} {
		if !slicesContains(got, want) {
			t.Errorf("Schemes() = %v, missing %q", got, want)
		}
	}

	// Sorted, so an error message listing them reads the same every run.
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Errorf("Schemes() is not sorted: %v", got)

			break
		}
	}
}

func slicesContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}

	return false
}

// TestUnregisteredSchemeSaysWhichAreRegistered pins the error's content, not
// just that there is one. "unknown scheme" sends a reader nowhere; the list of
// what *is* registered points straight at the missing import.
func TestUnregisteredSchemeSaysWhichAreRegistered(t *testing.T) {
	t.Parallel()

	_, err := file.OpenFS("nosuchscheme://host/path")
	if err == nil {
		t.Fatal("an unregistered scheme opened")
	}

	if !errors.Is(err, file.ErrNoScheme) {
		t.Errorf("error is %v, want it to wrap ErrNoScheme", err)
	}

	if !strings.Contains(err.Error(), "file") {
		t.Errorf("the error does not list the registered schemes: %v", err)
	}
}

// TestOpenFSDoesNotLeakCredentials is why redact exists.
//
// An sftp:// or s3:// URL can carry a password, and an error string ends up in
// logs, issue reports and CI output — none of which anyone thinks of as a place
// secrets go until one is there. The username survives because it is diagnostic
// and not a secret.
func TestOpenFSDoesNotLeakCredentials(t *testing.T) {
	t.Parallel()

	// A registered scheme whose open fails, so the error path that formats the
	// URL is the one taken. A file:// URL pointing at a path that cannot be
	// opened does it without needing a network backend.
	const secret = "hunter2"

	missing := filepath.ToSlash(filepath.Join(t.TempDir(), "definitely", "not", "here"))
	if !strings.HasPrefix(missing, "/") {
		missing = "/" + missing
	}

	_, err := file.OpenFS("file://astronomer:" + secret + "@localhost" + missing)
	if err == nil {
		t.Skip("this platform opened a directory that does not exist; nothing to redact")
	}

	if strings.Contains(err.Error(), secret) {
		t.Errorf("the error carries the password: %v", err)
	}

	if !strings.Contains(err.Error(), "astronomer") {
		t.Logf("the username was dropped as well as the password: %v", err)
	}
}

// TestPrefixScopesTheFilesystem covers ?prefix=, which is how an endpoint is
// pointed at a nested mirror without every caller prepending the same segment.
func TestPrefixScopesTheFilesystem(t *testing.T) {
	t.Parallel()

	_, fsURL := seedDir(t, map[string]string{
		"pub/naif/de440s.bsp": "kernel bytes",
		"decoy.txt":           "not this one",
	})

	fsys, err := file.OpenFS(fsURL + "&prefix=pub/naif/")
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	got, err := fs.ReadFile(fsys, "de440s.bsp")
	if err != nil {
		t.Fatalf("ReadFile through a prefix: %v", err)
	}

	if string(got) != "kernel bytes" {
		t.Errorf("read %q, want %q", got, "kernel bytes")
	}

	// And the prefix is a floor, not a hint: what is above it is unreachable.
	if _, err := fs.ReadFile(fsys, "decoy.txt"); err == nil {
		t.Error("a name outside the prefix was readable, so ?prefix= is not scoping")
	}

	// A trailing slash is the natural way to write a prefix and must not change
	// the meaning — fs.Sub would refuse the path with one.
	bare, err := file.OpenFS(fsURL + "&prefix=pub/naif")
	if err != nil {
		t.Fatalf("OpenFS without a trailing slash: %v", err)
	}

	if _, err := fs.ReadFile(bare, "de440s.bsp"); err != nil {
		t.Errorf("a prefix without a trailing slash behaves differently: %v", err)
	}
}

// TestKeyServesOneObjectUnderAnyName covers ?key=.
//
// The case it exists for: an endpoint that is one file rather than a directory.
// A caller still passes a name — remote.GetFile's signature requires one — and
// with a single-object URL there is nothing for that name to mean, so the
// wrapper makes every name resolve to the one object.
func TestKeyServesOneObjectUnderAnyName(t *testing.T) {
	t.Parallel()

	_, fsURL := seedDir(t, map[string]string{
		"eop/finals2000A.all": "MJD DUT1 ...",
	})

	fsys, err := file.OpenFS(fsURL + "&key=eop/finals2000A.all")
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	// Two different names, one object — which is the whole contract.
	for _, name := range []string{"finals2000A.data", "anything-at-all"} {
		got, rerr := fs.ReadFile(fsys, name)
		if rerr != nil {
			t.Errorf("ReadFile(%q): %v", name, rerr)

			continue
		}

		if string(got) != "MJD DUT1 ..." {
			t.Errorf("ReadFile(%q) = %q, want the single object's content", name, got)
		}
	}
}

// TestOpenFSCachesOneFilesystemPerURL pins the caching, including that two URLs
// differing only in their query are different filesystems — they have to be,
// since the query is what applies the wrappers above.
func TestOpenFSCachesOneFilesystemPerURL(t *testing.T) {
	t.Parallel()

	_, fsURL := seedDir(t, map[string]string{"a/b.txt": "x"})

	// A trailing parameter rather than a leading one: testutil.FileURL already
	// sets create_dir, so "?prefix=" here would make the second "?" part of the
	// first parameter's value and the wrapper would silently not apply — which
	// is exactly what this test caught the first time it was written.

	first, err := file.OpenFS(fsURL)
	if err != nil {
		t.Fatal(err)
	}

	again, err := file.OpenFS(fsURL)
	if err != nil {
		t.Fatal(err)
	}

	if first != again {
		t.Error("OpenFS returned two filesystems for one URL")
	}

	scoped, err := file.OpenFS(fsURL + "&prefix=a")
	if err != nil {
		t.Fatal(err)
	}

	if scoped == first {
		t.Error("a ?prefix= URL returned the unscoped filesystem; the query is not part " +
			"of the cache key")
	}
}

// uncancellable is an fs.FS that does not implement ContextFS, standing in for
// a backend that cannot be cancelled.
type uncancellable struct{ fs.FS }

// cancellable is the opposite: a filesystem that carries a context and refuses
// to read once it is done. A fake rather than a real backend, because the seam
// is what is under test — WithContext must hand the context to the value and
// the value's operations must see it, whatever the backend does with it.
type cancellable struct {
	fs.FS

	ctx context.Context //nolint:containedctx // the io/fs contract has nowhere else to put it
}

func (c cancellable) WithContext(ctx context.Context) fs.FS {
	clone := c
	clone.ctx = ctx

	return clone
}

func (c cancellable) Open(name string) (fs.File, error) {
	if c.ctx != nil {
		if err := c.ctx.Err(); err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: err}
		}
	}

	return c.FS.Open(name) //nolint:wrapcheck // already a *fs.PathError
}

// TestWithContextBindsWhatItCanAndLeavesTheRest covers both halves of the
// deliberate asymmetry between WithContext and RequireContext.
//
// WithContext is forgiving because a filesystem with nothing to cancel is a
// real and correct thing. RequireContext is not, because the paths that use it
// transfer for minutes and "the caller pressed ctrl-C and nothing happened" is
// a defect rather than an inconvenience.
func TestWithContextBindsWhatItCanAndLeavesTheRest(t *testing.T) {
	t.Parallel()

	_, fsURL := seedDir(t, map[string]string{"k.bsp": "bytes"})

	fsys, err := file.OpenFS(fsURL)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("a backend that cannot be cancelled is returned unchanged", func(t *testing.T) {
		t.Parallel()

		plain := uncancellable{FS: fsys}

		if bound := file.WithContext(t.Context(), plain); bound != fs.FS(plain) {
			t.Error("WithContext replaced a filesystem that does not implement ContextFS")
		}

		if _, err := file.RequireContext(t.Context(), plain); !errors.Is(err, file.ErrNoContext) {
			t.Errorf("RequireContext accepted it (%v); a Downloadable endpoint would get an "+
				"uncancellable transfer", err)
		}
	})

	t.Run("a cancelled context reaches the operation", func(t *testing.T) {
		t.Parallel()

		src := cancellable{FS: fsys}

		bound, err := file.RequireContext(t.Context(), src)
		if err != nil {
			t.Fatalf("RequireContext: %v", err)
		}

		if _, err := fs.ReadFile(bound, "k.bsp"); err != nil {
			t.Fatalf("a bound filesystem cannot read: %v", err)
		}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		dead, err := file.RequireContext(ctx, src)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := fs.ReadFile(dead, "k.bsp"); !errors.Is(err, context.Canceled) {
			t.Errorf("reading through a cancelled context gave %v, want context.Canceled — "+
				"WithContext is not handing the context to the filesystem", err)
		}

		// The receiver is untouched, so one registered filesystem serves any
		// number of callers with different deadlines.
		if _, err := fs.ReadFile(src, "k.bsp"); err != nil {
			t.Errorf("binding a context mutated the original filesystem: %v", err)
		}
	})
}
