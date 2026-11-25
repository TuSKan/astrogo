package remote

import (
	"net"
	"net/url"
	"strings"
)

// reachesNetwork reports whether a request to rawURL leaves this machine:
// anything but a file:// path, an in-memory filesystem, or an http(s) server
// on the loopback interface. A file:// URL naming a host is a network share,
// and a URL that does not parse is taken to reach the network, so offline mode
// refuses what it cannot place.
func reachesNetwork(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}

	switch strings.ToLower(u.Scheme) {
	case "mem":
		return false
	case "file":
		return u.Host != "" && !strings.EqualFold(u.Host, "localhost")
	case "http", "https":
		host := u.Hostname()
		if strings.EqualFold(host, "localhost") {
			return false
		}

		ip := net.ParseIP(host)

		return ip == nil || !ip.IsLoopback()
	default:
		return true
	}
}
