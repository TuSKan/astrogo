package azure_test

import (
	"testing"

	"gocloud.dev/blob"

	_ "github.com/TuSKan/astrogo/remote/file/azure"
)

// The whole contract of this package is "blank-importing it makes azblob://
// openable by remote". Opening a real bucket would need credentials and a
// network, so the scheme registration itself is what's asserted.
func TestBlankImportRegistersAzureScheme(t *testing.T) {
	t.Parallel()

	if !blob.DefaultURLMux().ValidBucketScheme("azblob") {
		t.Fatalf("azblob scheme not registered; available: %v", blob.DefaultURLMux().BucketSchemes())
	}
}
