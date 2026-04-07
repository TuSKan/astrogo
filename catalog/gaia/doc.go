// Package gaia provides a precise provider.ConeSearcher implementation over ESA's Gaia DR3.
//
// Gaia DR3 contains astrometric parameters (positions, parallaxes, proper motions) for
// nearly two billion stellar point sources. This package streams requests across the
// European Space Agency (ESA) endpoints using native ADQL CIRCLE operations mapping
// strict ICRS results on-the-fly.
package gaia
