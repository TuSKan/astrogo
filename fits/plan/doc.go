// Package plan bridges FITS header metadata to astrogo's planning types
// ([github.com/TuSKan/astrogo/plan.Site], [github.com/TuSKan/astrogo/plan.Observable]).
// It is a separate package specifically so that importing it — and its
// transitive dependency on FITS binary-table/image support (and, through
// that, Apache Arrow) — is opt-in: the core plan package itself has no
// FITS dependency.
package plan
