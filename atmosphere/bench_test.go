package atmosphere

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// ── Refraction Model Benchmarks ──────────────────────────────────────────────

func BenchmarkRefractionRigorous_FromTrue(b *testing.B) {
	model := RefractionRigorous{}
	env := StandardRefraction
	alt := angle.Deg(30)

	for b.Loop() {
		_ = model.RefractFromTrue(alt, env)
	}
}

func BenchmarkRefractionRigorous_FromApparent(b *testing.B) {
	model := RefractionRigorous{}
	env := StandardRefraction
	alt := angle.Deg(30)

	for b.Loop() {
		_ = model.RefractFromApparent(alt, env)
	}
}

func BenchmarkRefractionApproximate_FromTrue(b *testing.B) {
	model := RefractionApproximate{}
	env := StandardRefraction
	alt := angle.Deg(30)

	for b.Loop() {
		_ = model.RefractFromTrue(alt, env)
	}
}

func BenchmarkRefractionRigorous_Horizon(b *testing.B) {
	model := RefractionRigorous{}
	env := StandardRefraction
	alt := angle.Deg(0) // worst case: horizon

	for b.Loop() {
		_ = model.RefractFromTrue(alt, env)
	}
}

func BenchmarkAirmass(b *testing.B) {
	alt := angle.Deg(30)

	for b.Loop() {
		_, _ = Airmass(alt)
	}
}

func BenchmarkAtAltitude(b *testing.B) {
	for b.Loop() {
		_ = AtAltitude(2635)
	}
}
