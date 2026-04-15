// Package resolve provides the core types and HTTP infrastructure for
// astronomical target resolution across catalog providers.
//
// It defines the [Target] aggregate, the pluggable [Provider] interface,
// the [Client] HTTP transport with automatic retry and rate limiting,
// and the [Cache] for persisting resolved results locally.
package resolve
