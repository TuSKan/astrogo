package metrology

// Status is the outcome of a suite, as it appears in a report.
type Status string

// The three outcomes. There is deliberately no "skipped" that reads as
// success and no absence that reads as anything at all.
const (
	// StatusVerified means the suite ran and every sample sat inside its
	// contract.
	StatusVerified Status = "VERIFIED"

	// StatusViolated means the suite ran and at least one sample did not.
	StatusViolated Status = "CONTRACT_VIOLATED"

	// StatusNotVerified means the suite did not run — an unreachable
	// endpoint, a missing kernel, absent reference data.
	//
	// It exists so that not running is recorded rather than merely absent.
	// Every network-tagged suite here skips when its service is down, which
	// keeps builds honest about somebody else's outage; without this state
	// the accuracy record would keep showing the last good numbers with
	// nothing to say they are stale, and a service that went away
	// permanently would look indistinguishable from one that keeps passing.
	StatusNotVerified Status = "NOT_VERIFIED"
)

// OK reports whether the status represents a suite that ran and passed.
// StatusNotVerified is not OK: it is an absence of evidence, and a caller
// deciding whether to publish a number must not treat it as one.
func (s Status) OK() bool { return s == StatusVerified }
