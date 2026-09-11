package gcs_test

import (
	"testing"

	"gocloud.dev/blob"

	_ "github.com/TuSKan/astrogo/remote/file/gcs"
)

// The whole contract of this package is "blank-importing it makes gs://
// openable by remote". Opening a real bucket would need credentials and a
// network, so the scheme registration itself is what's asserted.
func TestBlankImportRegistersGCSScheme(t *testing.T) {
	t.Parallel()

	if !blob.DefaultURLMux().ValidBucketScheme("gs") {
		t.Fatalf("gs scheme not registered; available: %v", blob.DefaultURLMux().BucketSchemes())
	}
}
