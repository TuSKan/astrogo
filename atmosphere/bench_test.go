package atmosphere

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// ── Refraction Model Benchmarks ──────────────────────────────────────────────

func BenchmarkRefractionBennett_FromTrue(b *testing.B) {
	model := RefractionBennett{}
	env := StandardRefraction()
	alt := angle.Deg(30)

	for b.Loop() {
		_ = model.RefractFromTrue(alt, env)
	}
}

func BenchmarkRefractionBennett_FromApparent(b *testing.B) {
	model := RefractionBennett{}
	env := StandardRefraction()
	alt := angle.Deg(30)

	for b.Loop() {
		_ = model.RefractFromApparent(alt, env)
	}
}

func BenchmarkRefractionBennett_Horizon(b *testing.B) {
	model := RefractionBennett{}
	env := StandardRefraction()
	alt := angle.Deg(0) // worst case: horizon

	for b.Loop() {
		_ = model.RefractFromTrue(alt, env)
	}
}

func BenchmarkRefractionSOFA_FromTrue(b *testing.B) {
	model := RefractionSOFA{}
	env := StandardRefraction()
	alt := angle.Deg(30)

	for b.Loop() {
		_ = model.RefractFromTrue(alt, env)
	}
}

func BenchmarkRefractionSOFA_Horizon(b *testing.B) {
	model := RefractionSOFA{}
	env := StandardRefraction()
	alt := angle.Deg(0) // the hand-over to Bennett-NA, inverted

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
