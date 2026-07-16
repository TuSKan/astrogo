package iers

import (
	"testing"
	"testing/fstest"
)

// sampleFinals2000A mimics finals2000A.all format for two consecutive days
// (same fixture shape as reader_test.go's TestParseFinals2000A).
const sampleFinals2000A = `73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300   .143000   .137000   .8075000   -18.637    -3.667
73 1 3 41685.00 I  0.118980 0.011039  0.135656 0.013616  I 0.8056163 0.0002710  3.5563 0.1916  P    -0.751    0.199    -0.701    0.300   .141000   .134000   .8044000   -18.636    -3.571  `

func TestLoadFS(t *testing.T) {
	t.Cleanup(func() { RegisterModel(ZeroModel{}) })

	fsys := fstest.MapFS{
		"finals2000A.all": {Data: []byte(sampleFinals2000A)},
	}

	if err := LoadFS(fsys, "finals2000A.all"); err != nil {
		t.Fatalf("LoadFS: %v", err)
	}

	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after LoadFS")
	}

	if err := LoadFS(fsys, "does-not-exist.all"); err == nil {
		t.Error("expected an error for a missing FS entry")
	}
}
