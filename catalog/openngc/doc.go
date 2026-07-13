// Package openngc provides an embedded [resolve.Provider] for the OpenNGC catalog.
//
// The full NGC/IC dataset is compiled into the binary via go:embed, enabling
// zero-I/O offline resolution of deep-sky objects by name, NGC/IC number,
// Messier designation, or common alias — but only once `go generate
// ./catalog/openngc/...` has produced data/openngc.csv; that file is
// gitignored (never committed — see the README's "Data downloads &
// offline usage"), so a build from a fresh `go get` without running
// generate yields an empty, warning-logged provider from [New].
//
// The go:generate directive (openngc.go) pins its two upstream source URLs
// to a specific OpenNGC commit SHA, so regeneration is reproducible:
// running it twice against the same SHA always produces byte-identical
// output. Bump the pinned SHA (and re-run go generate) as a deliberate,
// reviewed data-update step — see
// https://github.com/mattiaverga/OpenNGC/commits/master for upstream's
// latest.
package openngc
