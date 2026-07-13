// Package resolve provides the core domain types for astronomical target
// resolution across catalog providers.
//
// It defines the [Target] aggregate, the pluggable [Provider] interface,
// and the [Cache] for persisting resolved results locally. Providers reach
// remote services through [github.com/TuSKan/astrogo/remote]'s [Client] and
// endpoint registry — see that package for HTTP transport, retry, and
// download-consent configuration.
package resolve
