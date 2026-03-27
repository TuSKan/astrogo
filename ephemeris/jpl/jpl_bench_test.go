package jpl_test

import (
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
)

func BenchmarkEvalChebyshev(b *testing.B) {
	coeffs := make([]float64, 14) // Typical length for planets
	for i := range coeffs {
		coeffs[i] = 0.1 * float64(i)
	}
	tau := 0.5
	radius := 100.0

	b.Run("PositionOnly", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			spk.EvalChebyshev(coeffs, tau, radius, false)
		}
	})

	b.Run("WithDerivative", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			spk.EvalChebyshev(coeffs, tau, radius, true)
		}
	})
}

func BenchmarkFindSegment(b *testing.B) {
	p, _ := jpl.NewProvider(jpl.WithDataDir("data"))
	
	// Mock 100 kernels, each with 10 targets, each target has 1 segment.
	// Total 1,000 segments in Index.
	for k := 0; k < 100; k++ {
		kernel := &jpl.Kernel{
			Segments: make([]spk.Segment, 10),
		}
		for s := 0; s < 10; s++ {
			targetID := int32(k*10 + s)
			kernel.Segments[s] = spk.Segment{
				Target:  targetID,
				StartET: -1e15,
				EndET:   1e15,
			}
			ref := jpl.SegmentRef{KernelIndex: k, SegmentIndex: s}
			p.Index = append(p.Index, ref)
			if p.ByTarget[targetID] == nil {
				p.ByTarget[targetID] = make([]jpl.SegmentRef, 0)
			}
			p.ByTarget[targetID] = append(p.ByTarget[targetID], ref)
			
			// Update coverage
			p.ByTargetCoverage[targetID] = jpl.TargetCoverage{
				StartET: -1e15,
				EndET:   1e15,
				Count:   1,
			}
		}
		p.Kernels = append(p.Kernels, kernel)
	}

	// Search for a target that was loaded EARLY (e.g., target 5)
	// Global scan will have to check ~995 segments.
	// Target scan will only check 1 segment.
	const searchTarget = int32(5)

	b.Run("GlobalScan", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			et := 0.0
			found := false
			for j := len(p.Index) - 1; j >= 0; j-- {
				ref := p.Index[j]
				seg := &p.Kernels[ref.KernelIndex].Segments[ref.SegmentIndex]
				if seg.Target == searchTarget && et >= seg.StartET && et <= seg.EndET {
					found = true
					break
				}
			}
			if !found {
				b.Fatal("not found")
			}
		}
	})

	b.Run("TargetScan", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := p.FindSegment(searchTarget, 0.0)
			if err != nil {
				b.Fatalf("failed: %v", err)
			}
		}
	})
}
