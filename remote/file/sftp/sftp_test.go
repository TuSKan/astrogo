package sftp_test

import (
	"testing"

	"gocloud.dev/blob"

	_ "github.com/TuSKan/astrogo/remote/file/sftp"
)

// The whole contract of this package is "blank-importing it makes sftp://
// openable by remote". Opening a real bucket would need credentials and a
// network, so the scheme registration itself is what's asserted.
func TestBlankImportRegistersSFTPScheme(t *testing.T) {
	t.Parallel()

	if !blob.DefaultURLMux().ValidBucketScheme("sftp") {
		t.Fatalf("sftp scheme not registered; available: %v", blob.DefaultURLMux().BucketSchemes())
	}
}
