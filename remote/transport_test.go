package remote

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	gofs "github.com/ungerik/go-fs"
)

// scratchKind is a Kind used only by this file's tests — never registered
// as a default endpoint Kind, so it never collides with KindAPI/KindFile/
// KindS3's real transports. Deliberately a var, not a const: exhaustive
// treats every package-level Kind-typed const as an enum member requiring
// a switch case (e.g. TestDefaultEndpointsHaveExplicitTimeouts's switch on
// ep.Kind), which a test-only scratch value has no business being counted
// among.
var scratchKind = Kind("scratch-test-kind")

// stubTransport is a minimal Transport double for exercising
// RegisterTransport/transportFor without any real network/S3 dependency.
type stubTransport struct{ id int }

func (s stubTransport) FetchInto(context.Context, EndpointID, string, gofs.File, time.Duration, func([]byte) error, func(downloaded, total int64)) error {
	return nil
}

func (stubTransport) Probe(context.Context, EndpointID, string) (Signature, error) {
	return Signature{}, nil
}

// cleanTransports restores the transports map to its built-in-only state
// after a test that calls RegisterTransport — transportFor's registry is
// process-global and Reset() deliberately never touches it (see
// RegisterTransport's own doc comment), so a test that mutates it must
// clean up explicitly.
func cleanTransports(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		transportsMu.Lock()
		defer transportsMu.Unlock()

		transports = map[Kind]Transport{
			KindAPI:  httpTransport{},
			KindFile: httpTransport{},
		}
	})
}

func TestTransportForBuiltins(t *testing.T) {
	for _, k := range []Kind{KindAPI, KindFile} {
		tr, err := transportFor(k)
		if err != nil {
			t.Fatalf("transportFor(%s): %v", k, err)
		}

		if _, ok := tr.(httpTransport); !ok {
			t.Errorf("transportFor(%s) = %T, want httpTransport", k, tr)
		}
	}
}

func TestTransportForUnregisteredKind(t *testing.T) {
	if _, err := transportFor(scratchKind); !errors.Is(err, ErrNoTransport) {
		t.Errorf("transportFor(unregistered) error = %v, want ErrNoTransport", err)
	}
}

func TestRegisterTransportLastWins(t *testing.T) {
	cleanTransports(t)

	RegisterTransport(scratchKind, stubTransport{id: 1})
	RegisterTransport(scratchKind, stubTransport{id: 2})

	tr, err := transportFor(scratchKind)
	if err != nil {
		t.Fatalf("transportFor: %v", err)
	}

	got, ok := tr.(stubTransport)
	if !ok {
		t.Fatalf("transportFor(%s) = %T, want stubTransport", scratchKind, tr)
	}

	if got.id != 2 {
		t.Errorf("RegisterTransport should let the second call win, got id=%d", got.id)
	}
}

func TestRegisterTransportConcurrentRaceClean(t *testing.T) {
	cleanTransports(t)

	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(2)

		go func(i int) {
			defer wg.Done()

			RegisterTransport(scratchKind, stubTransport{id: i})
		}(i)

		go func() {
			defer wg.Done()

			_, _ = transportFor(scratchKind)
		}()
	}

	wg.Wait()

	if _, err := transportFor(scratchKind); err != nil {
		t.Errorf("transportFor after concurrent registration: %v", err)
	}
}

func TestResetDoesNotClearTransportRegistrations(t *testing.T) {
	cleanTransports(t)

	RegisterTransport(scratchKind, stubTransport{id: 7})
	Reset()

	tr, err := transportFor(scratchKind)
	if err != nil {
		t.Fatalf("transportFor after Reset: %v", err)
	}

	if _, ok := tr.(stubTransport); !ok {
		t.Errorf("Reset must not clear transport registrations; got %T", tr)
	}
}

// TestGetFileUnregisteredTransportFails is the machine proof that remote
// alone never dials a KindS3 endpoint: without importing remote/s3 (or
// registering any other Transport for KindS3), GetFile must fail with
// ErrNoTransport before ever attempting a fetch, and must not create a
// cache file — i.e. it fails on the transport lookup itself, not partway
// through a download attempt (matching TestGetFileDownloadDeniedWithoutConsent's
// own "no cache file on denial" convention).
func TestGetFileUnregisteredTransportFails(t *testing.T) {
	cleanRemoteState(t)

	EnableDownloads(CopernicusEODATA, 0)

	const key = "CAMS/GLOBAL/2023/01/01/some-file.nc"

	_, err := GetFile(context.Background(), CopernicusEODATA, key)
	if !errors.Is(err, ErrNoTransport) {
		t.Fatalf("GetFile with no registered KindS3 transport: err = %v, want ErrNoTransport", err)
	}

	dir, dirErr := CacheDir(CopernicusEODATA)
	if dirErr != nil {
		t.Fatalf("CacheDir: %v", dirErr)
	}

	if dir.Join(key).Exists() {
		t.Error("GetFile must not create a cache file when no transport is registered")
	}
}
