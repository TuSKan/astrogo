package core

import (
	"context"
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// The registry is process-global by design — one backend, installed by an init
// — so every test here restores it. They run in this package's own binary, so
// nothing else is looking at it.
func restoreBackend(t *testing.T) {
	t.Helper()

	prev := kernelBackend.Load()

	t.Cleanup(func() { kernelBackend.Store(prev) })
}

// Sentinels for the failures these tests inject, declared here because a
// dynamic errors.New inside a test is the same footgun it is anywhere else:
// errors.Is against it works only by identity, and an inline one cannot be
// referred to twice.
var (
	errKernelGone = errors.New("de440.bsp: no such file")
	errFirst      = errors.New("first backend")
	errSecond     = errors.New("second backend")
)

// stubProvider is the smallest thing satisfying Provider.
type stubProvider struct{}

func (stubProvider) State(ID, time.Time) (State, error) {
	return State{Pos: vector.Vec3{X: 1}}, nil
}

func (stubProvider) Close() error { return nil }

// TestNoBackendReportsWhatToImport is the failure mode a build starts in, and
// the reason the whole split is acceptable: the message has to be actionable,
// because the alternative to this error used to be a compile error.
func TestNoBackendReportsWhatToImport(t *testing.T) {
	restoreBackend(t)
	RegisterKernelBackend(nil)

	if HasKernelBackend() {
		t.Fatal("HasKernelBackend is true after registering nil")
	}

	_, err := KernelProvider(context.Background(), KernelRequest{Source: Planets, Kernel: "de440"})
	if !errors.Is(err, ErrNoKernelBackend) {
		t.Fatalf("err = %v, want ErrNoKernelBackend", err)
	}
}

// TestRequestCrossesTheBoundaryIntact is why KernelRequest is a struct rather
// than a parameter list. The backend is registered by one package and called by
// another, so the compiler cannot keep a growing argument list in step — and a
// silently dropped time interval is a provider answering for the wrong window.
func TestRequestCrossesTheBoundaryIntact(t *testing.T) {
	restoreBackend(t)

	var got KernelRequest

	RegisterKernelBackend(func(_ context.Context, req KernelRequest) (Provider, error) {
		got = req
		return stubProvider{}, nil
	})

	if !HasKernelBackend() {
		t.Fatal("HasKernelBackend is false after registering a backend")
	}

	want := KernelRequest{
		Source:       SmallBody,
		Kernel:       "433",
		Start:        time.FromJD(2461000.5, time.TT),
		End:          time.FromJD(2461100.5, time.TT),
		ExtraKernels: []string{"de441_part-2"},
	}

	p, err := KernelProvider(context.Background(), want)
	if err != nil {
		t.Fatalf("KernelProvider: %v", err)
	}

	if p == nil {
		t.Fatal("KernelProvider returned a nil provider and no error")
	}

	switch {
	case got.Source != want.Source:
		t.Errorf("Source = %v, want %v", got.Source, want.Source)
	case got.Kernel != want.Kernel:
		t.Errorf("Kernel = %q, want %q", got.Kernel, want.Kernel)
	case !got.Start.Equal(want.Start):
		t.Errorf("Start = %v, want %v", got.Start, want.Start)
	case !got.End.Equal(want.End):
		t.Errorf("End = %v, want %v", got.End, want.End)
	case len(got.ExtraKernels) != 1 || got.ExtraKernels[0] != "de441_part-2":
		t.Errorf("ExtraKernels = %v, want [de441_part-2]", got.ExtraKernels)
	}
}

// TestBackendErrorReachesTheCaller: the registry is a lookup, not a policy. A
// backend that cannot open a kernel has to say so in its own words, or every
// real failure would arrive looking like a missing import.
func TestBackendErrorReachesTheCaller(t *testing.T) {
	restoreBackend(t)

	RegisterKernelBackend(func(context.Context, KernelRequest) (Provider, error) {
		return nil, errKernelGone
	})

	_, err := KernelProvider(context.Background(), KernelRequest{Source: Planets, Kernel: "de440"})
	if !errors.Is(err, errKernelGone) {
		t.Fatalf("err = %v, want the backend's own error", err)
	}

	if errors.Is(err, ErrNoKernelBackend) {
		t.Error("a backend's own failure was reported as a missing backend")
	}
}

// TestRegisterReplaces: last registration wins, and registering nil clears.
// Neither is something a caller should do, but the state is process-global and
// a half-cleared registry would be worse than either.
func TestRegisterReplaces(t *testing.T) {
	restoreBackend(t)

	RegisterKernelBackend(func(context.Context, KernelRequest) (Provider, error) {
		return nil, errFirst
	})
	RegisterKernelBackend(func(context.Context, KernelRequest) (Provider, error) {
		return nil, errSecond
	})

	_, err := KernelProvider(context.Background(), KernelRequest{})
	if !errors.Is(err, errSecond) {
		t.Errorf("err = %v, want the second registration to win", err)
	}

	RegisterKernelBackend(nil)

	if _, err := KernelProvider(context.Background(), KernelRequest{}); !errors.Is(err, ErrNoKernelBackend) {
		t.Errorf("err = %v after clearing, want ErrNoKernelBackend", err)
	}
}
