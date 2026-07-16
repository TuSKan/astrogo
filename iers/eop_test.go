package iers

import (
	"testing"
	"testing/fstest"
)

func TestGetModelLazyLoadDoesNotOverrideExplicitRegistration(t *testing.T) {
	t.Cleanup(func() { RegisterModel(ZeroModel{}) })

	fsys := fstest.MapFS{
		"finals2000A.all": {Data: []byte(sampleFinals2000A)},
	}

	// Explicit registration must win regardless of whether GetModel's lazy
	// embedded-load has already fired in this process.
	if err := LoadFS(fsys, "finals2000A.all"); err != nil {
		t.Fatal(err)
	}

	before, _, _ := Coverage()

	// A further GetModel call must not silently swap the model back to the
	// embedded snapshot.
	_ = GetModel()

	after, _, _ := Coverage()
	if before != after {
		t.Errorf("GetModel call mutated the explicitly-registered model: before=%v after=%v", before, after)
	}
}
