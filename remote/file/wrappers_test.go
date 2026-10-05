package file_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/TuSKan/astrogo/remote/file"
)

// capabilitiesOf names the optional interfaces fsys implements.
func capabilitiesOf(fsys fs.FS) map[string]bool {
	_, create := fsys.(file.CreateFS)
	_, excl := fsys.(file.CreateExclFS)
	_, remove := fsys.(file.RemoveFS)
	_, ctx := fsys.(file.ContextFS)

	return map[string]bool{"CreateFS": create, "CreateExclFS": excl, "RemoveFS": remove, "ContextFS": ctx}
}

func withQuery(u, q string) string {
	if strings.Contains(u, "?") {
		return u + "&" + q
	}

	return u + "?" + q
}

// TestQueryWrappersKeepTheBackendsCapabilities is #487: ?prefix= went through
// fs.Sub and ?key= through a struct with one Open method, and each hid every
// interface astrogo discovers by type assertion. A wrapper must now keep
// exactly what its backend has, and ?key= everything but the writes.
func TestQueryWrappersKeepTheBackendsCapabilities(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.FileServer(http.FS(fstest.MapFS{"pub/naif/k.bsp": {Data: []byte("k")}})))
	t.Cleanup(srv.Close)

	for _, base := range []string{
		seedDir(t, map[string]string{"pub/naif/k.bsp": "k"}),
		"mem://wrappers-keep-capabilities",
		srv.URL + "/",
	} {
		plain, err := file.OpenFS(base)
		if err != nil {
			t.Fatalf("OpenFS %s: %v", base, err)
		}

		want := capabilitiesOf(plain)
		if !want["ContextFS"] {
			t.Fatalf("%s: the backend itself is not a ContextFS, so this test checks nothing about keeping it", base)
		}

		prefixed, err := file.OpenFS(withQuery(base, "prefix=pub/naif/"))
		if err != nil {
			t.Fatalf("OpenFS %s with ?prefix=: %v", base, err)
		}

		if got := capabilitiesOf(prefixed); !mapsEqual(got, want) {
			t.Errorf("%s: ?prefix= has %v, its backend %v", base, got, want)
		}

		single, err := file.OpenFS(withQuery(base, "key=pub/naif/k.bsp"))
		if err != nil {
			t.Fatalf("OpenFS %s with ?key=: %v", base, err)
		}

		wantSingle := map[string]bool{"CreateFS": false, "CreateExclFS": false, "RemoveFS": false, "ContextFS": true}
		if got := capabilitiesOf(single); !mapsEqual(got, wantSingle) {
			t.Errorf("%s: ?key= has %v, want only ContextFS", base, got)
		}

		// And binding keeps them: a download binds before it reads.
		bound := file.WithContext(context.Background(), prefixed)
		if got := capabilitiesOf(bound); !mapsEqual(got, want) {
			t.Errorf("%s: ?prefix= bound to a context has %v, its backend %v", base, got, want)
		}
	}
}

func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}

	for k, v := range a {
		if b[k] != v {
			return false
		}
	}

	return true
}

// TestWritesThroughAPrefixLandUnderIt: a write, an exclusive create and a
// removal through ?prefix= act on the prefixed name, and an error names the
// caller's name rather than the backend's.
func TestWritesThroughAPrefixLandUnderIt(t *testing.T) {
	t.Parallel()

	base := seedDir(t, map[string]string{"keep.txt": "untouched"})

	plain, err := file.OpenFS(base)
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	scoped, err := file.OpenFS(withQuery(base, "prefix=cache/astrogo/"))
	if err != nil {
		t.Fatalf("OpenFS with ?prefix=: %v", err)
	}

	ctx := context.Background()

	if err := file.WriteFile(ctx, scoped, "eop/finals.data", bytes.NewReader([]byte("eop"))); err != nil {
		t.Fatalf("WriteFile through the prefix: %v", err)
	}

	if got, err := fs.ReadFile(plain, "cache/astrogo/eop/finals.data"); err != nil || string(got) != "eop" {
		t.Fatalf("the write landed elsewhere: read %q, %v", got, err)
	}

	w, err := scoped.(file.CreateExclFS).CreateExcl("eop/finals.data") //nolint:forcetypeassert // the capability test above holds this
	if err == nil {
		_ = w.Close()

		t.Fatal("CreateExcl through the prefix created over an existing object")
	}

	var pe *fs.PathError
	if !errors.Is(err, fs.ErrExist) || !errors.As(err, &pe) || pe.Path != "eop/finals.data" {
		t.Errorf("CreateExcl over an existing object returned %v, want fs.ErrExist naming eop/finals.data", err)
	}

	if err := file.Remove(ctx, scoped, "eop/finals.data"); err != nil {
		t.Fatalf("Remove through the prefix: %v", err)
	}

	if _, err := fs.Stat(plain, "cache/astrogo/eop/finals.data"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the object is still there after Remove through the prefix: %v", err)
	}

	// A name fs.Sub would refuse is refused for writing too, before it can
	// climb out of the prefix.
	if err := file.WriteFile(ctx, scoped, "../keep.txt", bytes.NewReader([]byte("clobbered"))); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("writing ../keep.txt through the prefix returned %v, want fs.ErrInvalid", err)
	}

	if got, _ := fs.ReadFile(plain, "keep.txt"); string(got) != "untouched" {
		t.Errorf("keep.txt above the prefix now reads %q", got)
	}
}

// partialFS can create but not create exclusively: no wrapper reproduces that
// set, so OpenFS must refuse it with ?prefix= rather than drop the create.
type partialFS struct{ fstest.MapFS }

func (partialFS) Create(string) (io.WriteCloser, error) { return nil, errors.ErrUnsupported }

func TestQueryWrapperRefusesToDropACapability(t *testing.T) {
	t.Parallel()

	file.Register("partialwrite487", func(*url.URL) (fs.FS, error) { return partialFS{fstest.MapFS{}}, nil })

	if _, err := file.OpenFS("partialwrite487://x?prefix=a/"); err == nil {
		t.Fatal("OpenFS returned a ?prefix= filesystem that silently lost CreateFS")
	} else if !strings.Contains(err.Error(), "CreateFS") {
		t.Errorf("the refusal does not say what would be lost: %v", err)
	}
}
