package testutil

import (
	"context"
	"strings"
	"testing"

	"gocloud.dev/blob"
)

// BucketKeys lists the object keys under prefix, relative to it, so a test
// can assert on what a cache actually contains without reading a
// directory. Driver bookkeeping is filtered out: fileblob writes a
// ".attrs" sidecar per object, which is per-object metadata rather than a
// cached artifact.
//
// The parameter is *blob.Bucket rather than the remote/file alias: they
// are the same type, and naming the alias would make remote/file's own
// in-package tests an import cycle.
func BucketKeys(tb testing.TB, bucket *blob.Bucket, prefix string) []string {
	tb.Helper()

	var keys []string

	iter := bucket.List(&blob.ListOptions{Prefix: prefix})

	for {
		obj, err := iter.Next(context.Background())
		if err != nil {
			break
		}

		if obj.IsDir || strings.HasSuffix(obj.Key, ".attrs") {
			continue
		}

		keys = append(keys, strings.TrimPrefix(obj.Key, prefix))
	}

	return keys
}
