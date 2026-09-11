package testutil_test

import (
	"errors"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// BucketKeys is test scaffolding, which is exactly the argument for testing it:
// it is the assertion two cache tests make, so a version that quietly returns
// nothing leaves both of them passing while checking nothing at all.
//
// fstest.MapFS stands in for a bucket here. The helper takes an fs.FS precisely
// so it need not know what is behind it — a *remote.Bucket satisfies the same
// interface — and using the standard library's own implementation keeps this
// test free of the storage layer it would otherwise have to import.

func TestBucketKeysReturnsKeysRelativeToThePrefix(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"iers/finals2000A.data": {Data: []byte("eop")},
		"iers/nested/other.dat": {Data: []byte("x")},
		"jpl/de440s.bsp":        {Data: []byte("kernel")},
	}

	got := testutil.BucketKeys(t, fsys, "iers/")
	slices.Sort(got)

	want := []string{"finals2000A.data", "nested/other.dat"}
	if !slices.Equal(got, want) {
		t.Errorf("BucketKeys = %v, want %v — keys come back relative to the prefix, "+
			"and a prefix scopes the listing", got, want)
	}
}

// A fileblob bucket writes a ".attrs" sidecar beside every object. It is
// per-object metadata, not a cached artifact, so a test counting what a cache
// holds must not see it — which is the whole reason this helper exists rather
// than callers listing the bucket themselves.
func TestBucketKeysHidesDriverSidecars(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"jpl/de440s.bsp":       {Data: []byte("kernel")},
		"jpl/de440s.bsp.attrs": {Data: []byte(`{"metadata":{}}`)},
	}

	got := testutil.BucketKeys(t, fsys, "jpl/")

	want := []string{"de440s.bsp"}
	if !slices.Equal(got, want) {
		t.Errorf("BucketKeys = %v, want %v — the driver's .attrs sidecar is not a cached object", got, want)
	}
}

// An empty prefix lists the whole bucket. The walk needs a directory name and
// "" is not one, so this is the case that would panic or return nothing if the
// root were passed through unchanged.
func TestBucketKeysWithNoPrefixListsEverything(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"a.dat":     {Data: []byte("1")},
		"sub/b.dat": {Data: []byte("2")},
	}

	got := testutil.BucketKeys(t, fsys, "")
	slices.Sort(got)

	want := []string{"a.dat", "sub/b.dat"}
	if !slices.Equal(got, want) {
		t.Errorf("BucketKeys = %v, want %v", got, want)
	}
}

// A prefix nothing has been written under is an empty result rather than a
// failure — the state most of these assertions start from, and the one a
// cache test checks before anything has populated it.
func TestBucketKeysUnderAnAbsentPrefixIsEmptyNotAFailure(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{"jpl/de440s.bsp": {Data: []byte("kernel")}}

	if got := testutil.BucketKeys(t, fsys, "nothing-here/"); len(got) != 0 {
		t.Errorf("BucketKeys under an unwritten prefix = %v, want none", got)
	}
}

// An empty bucket is the other side of the same case, and the one a
// download-denied test asserts: nothing was cached, and the helper has to be
// able to say so rather than fail.
func TestBucketKeysOnAnEmptyBucketIsEmpty(t *testing.T) {
	t.Parallel()

	if got := testutil.BucketKeys(t, fstest.MapFS{}, ""); len(got) != 0 {
		t.Errorf("BucketKeys on an empty bucket = %v, want none", got)
	}
}

// TestBucketKeysReportsAWalkFailureRatherThanAnEmptyList is the case that makes
// this helper safe to assert on.
//
// Its callers check what a cache contains, and "the listing broke" and "the
// cache is empty" are the same value if a failure is swallowed — so a broken
// walk would read as a passing emptiness check. It fails the test instead.
func TestBucketKeysReportsAWalkFailureRatherThanAnEmptyList(t *testing.T) {
	t.Parallel()

	fake := &fatalTB{TB: t}

	testutil.BucketKeys(fake, brokenFS{}, "jpl/")

	if !fake.failed {
		t.Error("a failing walk returned an empty list instead of failing the test")
	}
}

// brokenFS is a filesystem whose root lists one entry and then refuses to say
// anything more about it — the shape of a store that answers a listing and then
// drops the connection.
type brokenFS struct{}

func (brokenFS) Open(string) (fs.File, error) { return nil, errUnreadable }

func (brokenFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "jpl" {
		return []fs.DirEntry{brokenEntry{}}, nil
	}

	return nil, errUnreadable
}

// brokenEntry claims to be a directory, so the walk descends into it and hits
// the ReadDir failure above.
type brokenEntry struct{}

func (brokenEntry) Name() string               { return "sub" }
func (brokenEntry) IsDir() bool                { return true }
func (brokenEntry) Type() fs.FileMode          { return fs.ModeDir }
func (brokenEntry) Info() (fs.FileInfo, error) { return nil, errUnreadable }

// errUnreadable stands for a store that cannot answer.
var errUnreadable = errors.New("the store cannot be read")

// fatalTB records a Fatalf instead of ending the test that is checking for it.
type fatalTB struct {
	testing.TB

	failed bool
}

func (f *fatalTB) Helper() {}

func (f *fatalTB) Fatalf(string, ...any) { f.failed = true }
