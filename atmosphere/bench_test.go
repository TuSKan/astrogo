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

	for range b.N {
		_ = model.RefractFromTrue(alt, env)
	}
}

func BenchmarkRefractionRigorous_FromApparent(b *testing.B) {
	model := RefractionRigorous{}
	env := StandardAtmosphere
	alt := angle.Deg(30)

	b.ResetTimer()

	for range b.N {
		_ = model.RefractFromApparent(alt, env)
	}
}

func BenchmarkRefractionApproximate_FromTrue(b *testing.B) {
	model := RefractionApproximate{}
	env := StandardAtmosphere
	alt := angle.Deg(30)

	b.ResetTimer()

	for range b.N {
		_ = model.RefractFromTrue(alt, env)
	}
}

func BenchmarkRefractionRigorous_Horizon(b *testing.B) {
	model := RefractionRigorous{}
	env := StandardAtmosphere
	alt := angle.Deg(0) // worst case: horizon

	b.ResetTimer()

	for range b.N {
		_ = model.RefractFromTrue(alt, env)
	}
}

func BenchmarkAirmass(b *testing.B) {
	alt := angle.Deg(30)

	b.ResetTimer()

	for range b.N {
		_, _ = Airmass(alt)
	}
}

func BenchmarkAtAltitude(b *testing.B) {
	b.ResetTimer()

	for range b.N {
		_ = AtAltitude(2635)
	}
}
