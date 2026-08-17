package skybrightness

import (
	"fmt"
	"strings"
	"time"
)

// Provenance records the science behind one component: what model, from
// which publication, implementing which equations, over what validity
// domain, with what known approximations.
//
// The repository's rule is that a scientific coefficient carries its
// origin. This type is where a component states that origin once, in a
// form a caller can read at runtime rather than only in source comments,
// so a result can be explained without reading the implementation.
type Provenance struct {
	// Model names the implemented model, e.g. "Kocifaj, Bara & Falchi 2022
	// semi-analytic all-sky".
	Model string

	// Version distinguishes revisions of this implementation.
	Version string

	// PrimaryReference is the publication the equations come from.
	PrimaryReference string

	// SecondaryReferences are supporting or superseding publications.
	SecondaryReferences []string

	// Equations names the specific equations implemented, e.g.
	// "Eq. 1-5". Naming a paper is not enough: a reader needs to know
	// which part of it this code actually reproduces.
	Equations string

	// Datasets names the data products used, with versions.
	Datasets []DatasetRef

	// ValidityDomain states where the model is applicable — altitude
	// range, wavelength range, aerosol regime, phase angle, whatever the
	// literature bounds.
	ValidityDomain string

	// KnownApproximations lists deliberate departures from the primary
	// source, each of which is a real error-budget entry.
	KnownApproximations []string

	// ExpectedAccuracy states the accuracy claimed by the source or
	// measured by validation — never a guess.
	ExpectedAccuracy string
}

// DatasetRef identifies one data product and the exact version used, so a
// result can be reproduced.
type DatasetRef struct {
	Name    string
	Version string
	Source  string
}

// String renders the reference compactly.
func (d DatasetRef) String() string {
	if d.Version == "" {
		return d.Name
	}

	return d.Name + "@" + d.Version
}

// String renders the provenance as a one-line summary.
func (p Provenance) String() string {
	var b strings.Builder

	b.WriteString(p.Model)

	if p.Version != "" {
		b.WriteString(" v")
		b.WriteString(p.Version)
	}

	if p.PrimaryReference != "" {
		b.WriteString(" [")
		b.WriteString(p.PrimaryReference)

		if p.Equations != "" {
			b.WriteString(", ")
			b.WriteString(p.Equations)
		}

		b.WriteString("]")
	}

	return b.String()
}

// Reproducibility is everything needed to explain why two evaluations
// differ: library and model versions, the dataset versions in play, the
// atmospheric provider and its timestamp, the fidelity, and the spectral
// grid.
//
// A scientific user comparing two numbers needs this more than they need
// another decimal place on either of them.
type Reproducibility struct {
	// ModelVersion identifies the assembled model.
	ModelVersion string

	// Fidelity is the level the evaluation ran at.
	Fidelity Fidelity

	// Grid is the spectral axis used.
	Grid string

	// AtmosphereProvider names where the atmospheric state came from.
	AtmosphereProvider string

	// AtmosphereTime is the timestamp of that atmospheric state, which may
	// differ from the observation time when a forecast or an analysis is
	// used.
	AtmosphereTime time.Time

	// Datasets are every data product involved, with versions.
	Datasets []DatasetRef

	// Components lists the component provenance records, in evaluation
	// order.
	Components []Provenance
}

// String renders a compact reproducibility summary.
func (r Reproducibility) String() string {
	return fmt.Sprintf("model=%s fidelity=%s grid=%s components=%d datasets=%d",
		r.ModelVersion, r.Fidelity, r.Grid, len(r.Components), len(r.Datasets))
}
