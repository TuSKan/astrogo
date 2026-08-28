package skybrightness_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// TestMain pins Earth orientation to zero for every test in this package.
//
// # Why this is necessary, and why it was not obvious
//
// Because the golden tables were reproducible on a developer's machine and
// nowhere else. Zodiacal light is a function of solar elongation and
// moonlight of the Moon's position, so both read the Sun and Moon through
// [github.com/TuSKan/astrogo/ephemeris] — and that path resolves UT1 through
// IERS Earth orientation parameters, which load lazily from a cache that a
// machine either has or does not.
//
// A machine with the cache and a machine without it therefore computed
// different skies. The difference is small — a fraction of a second of UT1
// moves the Sun by milliarcseconds — but the golden lock is 1e-12 relative,
// and the measured divergence was **2.5e-06 on zodiacal radiance**: six
// orders of magnitude past the lock and nowhere near a rounding artefact.
// Locally every table passed; in CI all four failed on every platform.
//
// [time.RegisterModel] with [time.ZeroModel] is the documented way to ask for
// deterministic zero EOP, and it also marks the choice authoritative so the
// lazy load cannot silently replace it later. The tables below are generated
// under it, which makes them a property of this code rather than of whatever
// IERS data happens to be on disk.
//
// # Why the whole package rather than the golden tests alone
//
// Because the hazard is not specific to them. Any test here that evaluates a
// scene at a fixed instant and asserts a number has the same dependency, and
// registering per-test would leave the next one to rediscover this. The
// package's own rule is already that evaluation performs no I/O; ambient EOP
// data is I/O arriving through a side door.
func TestMain(m *testing.M) {
	time.RegisterModel(time.ZeroModel{})

	os.Exit(m.Run())
}
