package storage_test

import (
	"testing"
)

// Benchmarks esqueleto para comparar baseline vs. políticas futuras.
// Mantidos como skipped até haver integrações concretas.

func BenchmarkStorage_Get_Set_WithFuturePolicies(b *testing.B) {
	b.Skip("Benchmark placeholder até integração de políticas de admissão/evicção")
}

func BenchmarkStorage_HitRatio_Simulation(b *testing.B) {
	b.Skip("Benchmark placeholder de taxa de acerto (hit ratio)")
}
