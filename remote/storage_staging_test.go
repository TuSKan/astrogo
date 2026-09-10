package remote

import (
	"net/url"
	"testing"
)

// TestDataDirBucketURLAlwaysStagesInBucket covers the half of #241 that a caller who
// configured anything would otherwise not get.
//
// The default cache URL carries no_tmp_dir=1 because defaultDataDirURL builds
// it that way. A caller who relocated the cache — through SetDataDir or the
// ASTROGO_CACHE_DIR environment variable, which is the documented way and the
// one a deployed application is most likely to have used — supplies the whole
// URL, and theirs would not carry it.
//
// That is the worst shape a fix can have: correct for whoever did not
// configure anything, silently absent for whoever did, with a symptom that
// points at a rename in os.TempDir rather than at their own setting.
//
// It is dataDirBucketURL rather than DataDirURL that is asserted, because that
// is the split: DataDirURL hands back what the caller configured, verbatim —
// which is what makes it worth reading and logging — and this is the URL
// astrogo actually opens. TestDataDirEnvOverride pins the other half.
func TestDataDirBucketURLAlwaysStagesInBucket(t *testing.T) {
	// Not parallel: SetDataDir and the environment are process-wide.
	t.Cleanup(func() { SetDataDir("") })

	cases := []struct {
		name  string
		set   string
		want  string // the value no_tmp_dir must end up with, "" for absent
		other map[string]string
	}{
		{
			name:  "a bare file URL gains it",
			set:   "file:///tmp/astrogo",
			want:  "1",
			other: map[string]string{},
		},
		{
			name:  "an existing parameter is kept alongside",
			set:   "file:///tmp/astrogo?create_dir=true",
			want:  "1",
			other: map[string]string{"create_dir": "true"},
		},
		{
			name:  "a caller's own choice is not overridden",
			set:   "file:///tmp/astrogo?no_tmp_dir=0",
			want:  "0",
			other: map[string]string{},
		},
		{
			// s3blob rejects parameters it does not know, so adding this one
			// would turn a working configuration into an open error — and S3
			// does not stage through os.TempDir, so there is nothing to fix.
			name:  "a non-file scheme is untouched",
			set:   "s3://astrogo-cache?region=eu-west-1",
			want:  "",
			other: map[string]string{"region": "eu-west-1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			SetDataDir(tc.set)

			got := dataDirBucketURL()

			u, err := url.Parse(got)
			if err != nil {
				t.Fatalf("dataDirBucketURL() = %q, which does not parse: %v", got, err)
			}

			if v := u.Query().Get("no_tmp_dir"); v != tc.want {
				t.Errorf("dataDirBucketURL() = %q, no_tmp_dir = %q, want %q.\n"+
					"  fileblob stages every write through a temp file named from a clock "+
					"that does not advance on Windows; putting that file in the bucket rather "+
					"than os.TempDir is what stops two buckets colliding over one object "+
					"name (#241).", got, v, tc.want)
			}

			for k, want := range tc.other {
				if v := u.Query().Get(k); v != want {
					t.Errorf("dataDirBucketURL() = %q dropped %s=%s.\n"+
						"  Rewriting the URL must preserve every parameter the caller set; "+
						"create_dir in particular is what lets a first run open a cache "+
						"directory that does not exist yet.", got, k, want)
				}
			}
		})
	}
}

// TestDataDirBucketURLLeavesAMalformedURLAlone checks that the rewrite does not
// become the place a bad URL is reported.
//
// file.Open is where that belongs, with the caller's own string in the
// message. Repairing or rejecting it here would move the error somewhere the
// caller has no reason to look, and a URL this package cannot parse is one it
// has no business editing.
func TestDataDirBucketURLLeavesAMalformedURLAlone(t *testing.T) {
	// Not parallel: SetDataDir is process-wide.
	t.Cleanup(func() { SetDataDir("") })

	const malformed = "file:///tmp/astrogo?%zz"

	SetDataDir(malformed)

	if got := dataDirBucketURL(); got != malformed {
		t.Errorf("dataDirBucketURL() = %q, want %q unchanged.\n"+
			"  A URL that does not parse is passed through so file.Open reports it, "+
			"rather than being silently rewritten into a different broken URL.", got, malformed)
	}
}
