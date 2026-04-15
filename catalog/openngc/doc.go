// Package openngc provides an embedded [resolve.Provider] for the OpenNGC catalog.
//
// The full NGC/IC dataset is compiled into the binary via go:embed, enabling
// zero-I/O offline resolution of deep-sky objects by name, NGC/IC number,
// Messier designation, or common alias.
package openngc
