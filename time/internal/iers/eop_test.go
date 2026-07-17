package iers

import (
	"sync"
	"testing"
)

func TestGetModelDefaultsToZeroModel(t *testing.T) {
	t.Cleanup(func() { RegisterModel(ZeroModel{}) })

	RegisterModel(ZeroModel{})

	if _, ok := GetModel().(ZeroModel); !ok {
		t.Errorf("expected ZeroModel by default, got %T", GetModel())
	}

	if _, _, ok := Coverage(); ok {
		t.Error("expected Coverage ok=false for ZeroModel")
	}
}

func TestRegisterModelConcurrent(t *testing.T) {
	t.Cleanup(func() { RegisterModel(ZeroModel{}) })

	var wg sync.WaitGroup

	for range 20 {
		wg.Go(func() { RegisterModel(ZeroModel{}) })
		wg.Go(func() { _ = GetModel() })
	}

	wg.Wait()
}
