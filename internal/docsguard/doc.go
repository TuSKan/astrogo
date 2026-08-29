// Package docsguard holds tests that check the repository's prose against its
// code, rather than its code against itself.
//
// It carries no production symbols and exists only so those checks have a
// package to live in. Documentation is not usually testable, but some of its
// failure modes are: a version that has to be updated by hand, an example
// that no longer compiles, a claim about a symbol that has been renamed.
// Those go stale silently and, for scientific software, cost trust out of
// proportion to their size — a reader who finds one stale statement has no
// way to tell which of the accuracy figures beside it is also stale.
package docsguard
