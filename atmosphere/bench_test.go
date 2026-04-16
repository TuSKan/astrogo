package atmosphere

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// ── Refraction Model Benchmarks ──────────────────────────────────────────────

func BenchmarkRefractionRigorous_FromTrue(b *testing.B) {
	model := RefractionRigorous{}
	env := StandardAtmosphere
	alt := angle.Deg(30)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.RefractFromTrue(alt, env)
	}
}

func BenchmarkRefractionRigorous_FromApparent(b *testing.B) {
	model := RefractionRigorous{}
	env := StandardAtmosphere
	alt := angle.Deg(30)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.RefractFromApparent(alt, env)
	}
}

func BenchmarkRefractionApproximate_FromTrue(b *testing.B) {
	model := RefractionApproximate{}
	env := StandardAtmosphere
	alt := angle.Deg(30)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.RefractFromTrue(alt, env)
	}
}

func BenchmarkRefractionRigorous_Horizon(b *testing.B) {
	model := RefractionRigorous{}
	env := StandardAtmosphere
	alt := angle.Deg(0) // worst case: horizon

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.RefractFromTrue(alt, env)
	}
}

func BenchmarkAirmass(b *testing.B) {
	alt := angle.Deg(30)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Airmass(alt)
	}
}

func BenchmarkAtAltitude(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = AtAltitude(2635)
	}
}
