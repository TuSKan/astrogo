// Package plan bridges the sky-brightness engine to astrogo's planning
// types, supplying a [github.com/TuSKan/astrogo/plan.SkyDepth] that an
// observing plan can score or gate targets against.
//
// # Why this is a separate package
//
// So that the planning engine depends on no sky-brightness code at all. The
// core `plan` package declares the one-method interface it needs and nothing
// more; everything that makes a limiting magnitude — a radiance engine, five
// downloaded datasets, an instrument, a photometric system — lives on this
// side of the boundary and is imported only by a caller who wants it. That
// mirrors [github.com/TuSKan/astrogo/fits/plan], which exists for the same
// reason with Apache Arrow behind it.
//
// The previous attempt did the opposite: `plan` imported the sky-brightness
// engine directly and a bespoke import test kept the dependency from
// spreading further. There is no test to write here, because there is no
// import to police.
//
// # What it does not cover
//
// Visual observation. Turning a sky radiance into a naked-eye or eyepiece
// limiting magnitude needs a human contrast model — Crumey (2014) is the
// modern treatment — and this package implements the imaging path only. The
// tempting shortcut is the one to refuse: Schaefer's (1990) SQM-to-NELM
// conversion consumes a single V-band number, so routing a spectrum through
// it discards the spectrum in the first step. See docs/skybrightness.md §16.
package plan
