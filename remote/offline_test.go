package remote

import (
	"errors"
	"testing"
)

// offlineHost is a URL on a host that is not this machine. Offline mode
// refuses it before any request, and .invalid never resolves (RFC 2606), so a
// test that points an endpoint here and goes offline contacts nobody.
const offlineHost = "https://offline.invalid/"

// TestOfflineRefusesOnlyTheNetwork: offline mode keeps a program on its own
// machine, so it refuses an endpoint whose URL leaves it and nothing else. A
// file:// path, a mem:// filesystem and a server on the loopback interface are
// this machine; a LAN address, a hostname that only begins with "localhost", a
// file:// URL naming another host (a network share) and any scheme it cannot
// place are not.
func TestOfflineRefusesOnlyTheNetwork(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		url     string
		network bool
	}{
		{"file:///C:/astrogo/data/?create_dir=true", false},
		{"file:///var/cache/astrogo/", false},
		{"file://localhost/var/cache/astrogo/", false},
		{"mem://fixtures", false},
		{"http://127.0.0.1:52817/", false},
		{"https://127.0.0.2/tap/", false},
		{"http://[::1]:8080/", false},
		{"http://localhost:8080/", false},
		{"http://LOCALHOST/", false},

		{offlineHost, true},
		{"https://simbad.cds.unistra.fr/simbad/sim-tap/", true},
		{"http://10.0.0.1/", true},
		{"http://192.168.1.20:8080/", true},
		{"http://localhost.example.com/", true},
		{"file://fileserver/share/astrogo/", true},
		{"s3://bucket/prefix/?region=eu-west-1", true},
		{"sftp://data.example.org/kernels/", true},
		{"http://[::1", true},
	} {
		cl := NewClient()
		if err := cl.SetURL(OpenNGC, c.url); err != nil {
			t.Fatalf("SetURL(%q): %v", c.url, err)
		}

		cl.SetOffline(true)

		_, err := cl.URL(OpenNGC)
		if got := errors.Is(err, ErrOffline); got != c.network {
			t.Errorf("offline URL with the endpoint at %q: err %v, want refused %v", c.url, err, c.network)
		}

		// Online, nothing is refused: the classification is offline mode's
		// alone.
		cl.SetOffline(false)

		if _, err := cl.URL(OpenNGC); err != nil {
			t.Errorf("online URL with the endpoint at %q: %v", c.url, err)
		}
	}
}

// TestOfflineKeepsTheOtherRefusals: an unknown or disabled endpoint is refused
// for that reason, offline or not, and never as ErrOffline, which would send a
// caller to turn offline mode off for an endpoint that still would not answer.
func TestOfflineKeepsTheOtherRefusals(t *testing.T) {
	t.Parallel()

	cl := NewClient()
	cl.SetOffline(true)

	if _, err := cl.URL(EndpointID("nope.not.registered")); !errors.Is(err, ErrUnknownEndpoint) {
		t.Errorf("offline URL of an unknown endpoint: err %v, want ErrUnknownEndpoint", err)
	}

	cl.Disable(SIMBAD)

	if _, err := cl.URL(SIMBAD); !errors.Is(err, ErrEndpointDisabled) {
		t.Errorf("offline URL of a disabled endpoint: err %v, want ErrEndpointDisabled", err)
	}
}
