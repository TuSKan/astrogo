package remote_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// TestClientsHoldSeparatePolicies is the thing #114 was filed for.
//
// The issue's example is an HTTP handler that must never block on a download
// beside a background prefetcher whose whole job is to download. While this
// package held one set of globals that pair could not exist: SetOffline(true)
// stopped the prefetcher too, and EnableDownloads opened the gate for the
// handler.
//
// Each subtest below fails against the package as it was, which is what makes
// them worth having rather than restating the implementation.
func TestClientsHoldSeparatePolicies(t *testing.T) {
	// Not parallel: it reads Default, which every other remote test mutates.
	t.Run("offline is per client", func(t *testing.T) {
		serving := remote.NewClient(remote.WithOffline(true))
		prefetch := remote.NewClient()

		if _, err := serving.URL(remote.SIMBAD); !errors.Is(err, remote.ErrOffline) {
			t.Errorf("the offline client resolved a URL, err = %v, want ErrOffline.\n"+
				"  A component configured not to touch the network must not, whatever any "+
				"other component in the binary is doing.", err)
		}

		if _, err := prefetch.URL(remote.SIMBAD); err != nil {
			t.Errorf("the online client failed to resolve a URL: %v.\n"+
				"  One client going offline must not take the others with it — that is the "+
				"whole of #114.", err)
		}
	})

	t.Run("download consent is per client", func(t *testing.T) {
		granted := remote.NewClient(remote.WithDownloads(0, remote.IERSFinals2000A))
		denied := remote.NewClient()

		if ok, _ := granted.DownloadsEnabled(remote.IERSFinals2000A); !ok {
			t.Error("WithDownloads did not grant consent on the client that asked for it.")
		}

		if ok, _ := denied.DownloadsEnabled(remote.IERSFinals2000A); ok {
			t.Error("a second client inherited consent it never granted.\n" +
				"  Consent is the policy astrogo is strictest about; a new client starting " +
				"open would make NewClient a way around the standing rule that nothing is " +
				"fetched without being asked.")
		}
	})

	t.Run("an endpoint override is per client", func(t *testing.T) {
		const mirror = "https://mirror.example/simbad/"

		redirected := remote.NewClient(remote.WithEndpointURL(remote.SIMBAD, mirror))
		plain := remote.NewClient()

		got, err := redirected.URL(remote.SIMBAD)
		if err != nil {
			t.Fatalf("redirected client: %v", err)
		}

		if got != mirror {
			t.Errorf("URL = %q, want %q", got, mirror)
		}

		other, err := plain.URL(remote.SIMBAD)
		if err != nil {
			t.Fatalf("plain client: %v", err)
		}

		if other == mirror {
			t.Error("a second client saw an override it never set.\n" +
				"  Redirecting one endpoint at a test server is the most common reason to " +
				"want a client of one's own; leaking it defeats the point.")
		}
	})
}

// TestNewClientDoesNotDisturbDefault pins the compatibility half.
//
// Every package-level function still operates on [remote.Default], and the
// entire argument for this being a shape change rather than a break is that a
// caller who never constructs a Client cannot tell the difference. A
// constructor that reached into shared state would break that quietly, and
// only for programs that used both.
func TestNewClientDoesNotDisturbDefault(t *testing.T) {
	// Not parallel: it mutates Default.
	t.Cleanup(remote.Capture().Restore)

	remote.SetOffline(false)
	remote.EnableDownloads(1<<20, remote.IERSFinals2000A)

	_ = remote.NewClient(
		remote.WithOffline(true),
		remote.WithDownloads(0),
		remote.WithEndpointURL(remote.SIMBAD, "https://elsewhere.example/"),
	)

	if remote.Offline() {
		t.Error("constructing an offline Client put the default offline.")
	}

	ok, maxSize := remote.DownloadsEnabled(remote.IERSFinals2000A)
	if !ok || maxSize != 1<<20 {
		t.Errorf("the default's consent is now (%v, %d), want (true, %d).\n"+
			"  A new Client must not rewrite the policy the process already set.",
			ok, maxSize, 1<<20)
	}
}

// TestDefaultIsTheOneThePackageFunctionsUse checks the delegation itself.
//
// It would be entirely possible to add Client, wire the package functions to
// their own copy of the state, and have both work in isolation while meaning
// different things — which is the failure a caller would find only by mixing
// the two styles in one program.
func TestDefaultIsTheOneThePackageFunctionsUse(t *testing.T) {
	// Not parallel: it mutates Default.
	t.Cleanup(remote.Capture().Restore)

	remote.SetOffline(true)

	if !remote.Default().Offline() {
		t.Error("SetOffline did not reach Default().\n" +
			"  The package functions are documented as Default()'s methods; if they are " +
			"not, a program using both styles sees two policies where it configured one.")
	}

	remote.Default().SetOffline(false)

	if remote.Offline() {
		t.Error("Default().SetOffline did not reach the package function.")
	}
}
