package s3_test

import (
	"testing"

	"gocloud.dev/blob"

	_ "github.com/TuSKan/astrogo/remote/s3"
)

// The whole contract of this package is "blank-importing it makes s3://
// openable by remote/file". Opening a real bucket would need credentials
// and a network, so the scheme registration itself is what's asserted.
func TestBlankImportRegistersS3Scheme(t *testing.T) {
	t.Parallel()

	if !blob.DefaultURLMux().ValidBucketScheme("s3") {
		t.Fatalf("s3 scheme not registered; available: %v", blob.DefaultURLMux().BucketSchemes())
	}
}
