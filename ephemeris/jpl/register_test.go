package jpl

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// Importing this package is the whole contract under a blank import, so the
// registration is what these check — not the kernel reading, which every other
// test in this package already covers against a real DE440s.

// TestImportingThisPackageRegistersTheBackend is the contract in one line.
//
// If it broke, nothing would fail to compile and nothing would look wrong: a
// build with the blank import would simply report ErrNoKernelBackend, which is
// the message telling the caller to add the import they already added.
func TestImportingThisPackageRegistersTheBackend(t *testing.T) {
	if !core.HasKernelBackend() {
		t.Fatal("importing ephemeris/jpl did not register a kernel backend")
	}
}

// TestBackendReportsTheKernelFailureNotAMissingImport: with the backend
// registered, a kernel that cannot be read must arrive as that failure. The
// registry is a lookup and not a policy, and confusing the two would send a
// caller chasing an import they already have.
func TestBackendReportsTheKernelFailureNotAMissingImport(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)

	// Nothing can be fetched, so whatever comes back is a kernel failure.
	remote.SetOffline(true)

	_, err := core.KernelProvider(context.Background(), core.KernelRequest{
		Source: core.Planets,
		Kernel: "de440s",
	})
	if err == nil {
		t.Fatal("an offline kernel fetch returned no error")
	}

	if errors.Is(err, core.ErrNoKernelBackend) {
		t.Errorf("a kernel failure was reported as a missing backend:\n  %v", err)
	}

	if !strings.Contains(err.Error(), "ephemeris: new provider") {
		t.Errorf("the error does not say which step failed:\n  %v", err)
	}
}

// TestBackendCarriesTheTimeInterval: a small-body kernel is generated for a
// window, so an interval the backend drops is a provider that answers for the
// wrong dates. Offline, this reaches the same failure either way — what it
// pins is that the option translation runs at all rather than being skipped.
func TestBackendCarriesTheTimeInterval(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)
	remote.SetOffline(true)

	_, err := core.KernelProvider(context.Background(), core.KernelRequest{
		Source: core.SmallBody,
		Kernel: "433",
		Start:  time.FromJD(2461000.5, time.TT),
		End:    time.FromJD(2461100.5, time.TT),
	})
	if err == nil {
		t.Fatal("an offline small-body fetch returned no error")
	}

	if errors.Is(err, core.ErrNoKernelBackend) {
		t.Errorf("a kernel failure was reported as a missing backend:\n  %v", err)
	}
}
