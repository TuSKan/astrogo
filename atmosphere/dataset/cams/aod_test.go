package cams_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere/dataset/cams"
)

// A caller with no Copernicus account is told how to get one.
//
// # Why this is the whole of what can be offered
//
// The credentials cannot be supplied for them. They are issued per user,
// sharing them breaches the terms they were issued under, and anything done
// with a shared key traces back to whoever registered it — so shipping keys,
// encrypted or otherwise, would be handing every user someone else's
// liability along with an obfuscation anyone can undo. The URL is what is
// actually useful, which makes it worth pinning rather than leaving to drift
// out of the error nobody reads until they need it.
func TestRegistrationAdviceNamesWhatIsNeeded(t *testing.T) {
	t.Parallel()

	// Bound to a local because gocritic reads Contains(Const, variable) as a
	// reversed argument order; the constant here is genuinely the haystack.
	advice := cams.RegistrationAdvice

	for _, want := range []string{
		"https://dataspace.copernicus.eu",
		"AWS_ACCESS_KEY_ID",
		"remote/s3",
		"EnableDownloads",
	} {
		if !strings.Contains(advice, want) {
			t.Errorf("the advice does not mention %q, so a caller is told they cannot "+
				"proceed without being told how", want)
		}
	}
}
