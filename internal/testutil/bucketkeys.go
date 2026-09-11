package testutil

import (
	"io/fs"
	"path"
	"strings"
	"testing"
)

// BucketKeys lists the object keys under prefix, relative to it, so a test
// can assert on what a cache actually contains without reading a
// directory. Driver bookkeeping is filtered out: fileblob writes a
// ".attrs" sidecar per object, which is per-object metadata rather than a
// cached artifact.
//
// The parameter is fs.FS rather than a bucket type, which is what lets this
// helper exist at all: remote/file's own tests use it, so naming that package
// here would be an import cycle, and naming gocloud.dev/blob would put the
// storage driver in a package that has no business knowing which one astrogo
// uses. A *remote.Bucket satisfies it directly — the type implements fs.FS and
// fs.SubFS — so no call site changes and nothing is asserted about the backend
// beyond what the standard library already describes.
func BucketKeys(tb testing.TB, bucket fs.FS, prefix string) []string {
	tb.Helper()

	// fs.WalkDir needs a directory name, and "" is not one: the root is ".".
	// A bucket prefix conventionally ends in "/", which is not a valid fs
	// path element either, so it is trimmed for the walk and put back when
	// each key is reported.
	root := strings.TrimSuffix(prefix, "/")
	if root == "" {
		root = "."
	}

	var keys []string

	err := fs.WalkDir(bucket, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// A prefix nothing has been written under is an empty result,
			// not a failure — that is the state most of these assertions
			// start from.
			if p == root {
				return fs.SkipAll
			}

			return err
		}

		if d.IsDir() || strings.HasSuffix(p, ".attrs") {
			return nil
		}

		if root != "." {
			p = strings.TrimPrefix(p, root+"/")
		}

		keys = append(keys, p)

		return nil
	})
	if err != nil {
		tb.Fatalf("list keys under %q: %v", prefix, err)
	}

	// Keys are reported relative to prefix, and a prefix that named a
	// subdirectory has already been trimmed above; path.Clean here would
	// change nothing, so the only normalisation left is the separator, which
	// fs paths already use.
	for i, k := range keys {
		keys[i] = path.Clean(k)
	}

	return keys
}
