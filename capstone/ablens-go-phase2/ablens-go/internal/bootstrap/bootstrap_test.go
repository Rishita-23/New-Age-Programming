package bootstrap

import (
	"context"
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

func sample(seed uint64, n int, mean, sd float64) []float64 {
	r := rand.New(rand.NewPCG(seed, 1))
	xs := make([]float64, n)
	for i := range xs {
		xs[i] = mean + sd*r.NormFloat64()
	}
	return xs
}

func TestDeterministicAcrossWorkerCounts(t *testing.T) {
	c, tr := sample(1, 2000, 10, 3), sample(2, 2000, 10.5, 3)
	var ref Result
	for i, w := range []int{1, 2, 4, 8} {
		res, err := Run(context.Background(), c, tr, 500, w, 99, 0.05)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			ref = res
			continue
		}
		if res.CILow != ref.CILow || res.CIHigh != ref.CIHigh || res.StdErr != ref.StdErr {
			t.Errorf("workers=%d differs: %+v vs %+v", w, res, ref)
		}
	}
}

func TestDifferentSeedsDiffer(t *testing.T) {
	c, tr := sample(1, 1000, 10, 3), sample(2, 1000, 10.5, 3)
	a, _ := Run(context.Background(), c, tr, 400, 2, 1, 0.05)
	b, _ := Run(context.Background(), c, tr, 400, 2, 2, 0.05)
	if a.CILow == b.CILow && a.CIHigh == b.CIHigh {
		t.Error("different seeds produced identical intervals")
	}
}

func TestIntervalCoversTrueDifferenceAndMatchesTheory(t *testing.T) {
	c, tr := sample(3, 4000, 10, 2), sample(4, 4000, 10.4, 2)
	res, err := Run(context.Background(), c, tr, 2000, 4, 5, 0.05)
	if err != nil {
		t.Fatal(err)
	}
	if !(res.CILow < 0.4 && 0.4 < res.CIHigh) {
		t.Errorf("CI [%v, %v] does not cover 0.4", res.CILow, res.CIHigh)
	}
	wantSE := math.Sqrt(4.0/4000 + 4.0/4000)
	if math.Abs(res.StdErr-wantSE)/wantSE > 0.1 {
		t.Errorf("std err %v, theory %v", res.StdErr, wantSE)
	}
	if res.CILow >= res.CIHigh {
		t.Error("interval inverted")
	}
}

func TestPartialFinalChunk(t *testing.T) {
	c, tr := sample(1, 50, 1, 1), sample(2, 50, 1, 1)
	res, err := Run(context.Background(), c, tr, chunkSize+5, 3, 1, 0.05)
	if err != nil || res.Iterations != chunkSize+5 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestInputErrors(t *testing.T) {
	if _, err := Run(context.Background(), nil, []float64{1}, 10, 1, 1, 0.05); err == nil {
		t.Error("empty control")
	}
	if _, err := Run(context.Background(), []float64{1}, []float64{1}, 0, 1, 1, 0.05); err == nil {
		t.Error("zero iterations")
	}
}

func TestCancellation(t *testing.T) {
	c, tr := sample(1, 200000, 1, 1), sample(2, 200000, 1, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Run(ctx, c, tr, 100000, 2, 1, 0.05)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("cancellation took %v", time.Since(start))
	}
}

func benchmarkWorkers(b *testing.B, workers int) {
	c, tr := sample(1, 50000, 10, 3), sample(2, 50000, 10.2, 3)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Run(context.Background(), c, tr, 256, workers, 1, 0.05); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWorkers1(b *testing.B) { benchmarkWorkers(b, 1) }
func BenchmarkWorkers2(b *testing.B) { benchmarkWorkers(b, 2) }
func BenchmarkWorkers4(b *testing.B) { benchmarkWorkers(b, 4) }
