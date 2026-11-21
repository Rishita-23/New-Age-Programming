package agg

import (
	"math"
	"math/rand/v2"
	"testing"

	"ablens/internal/model"
)

func TestWelfordMatchesTwoPass(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 7))
	xs := make([]float64, 5000)
	var a Acc
	var sum float64
	for i := range xs {
		xs[i] = 1e6 + r.NormFloat64()*3
		sum += xs[i]
		a.Add(xs[i])
	}
	mean := sum / float64(len(xs))
	var ss float64
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	want := ss / float64(len(xs)-1)
	if math.Abs(a.Mean-mean) > 1e-6 || math.Abs(a.Variance()-want) > 1e-6 {
		t.Errorf("mean %v vs %v, var %v vs %v", a.Mean, mean, a.Variance(), want)
	}
}

func TestVarianceSmallN(t *testing.T) {
	var a Acc
	if a.Variance() != 0 {
		t.Error("empty variance")
	}
	a.Add(3)
	if a.Variance() != 0 {
		t.Error("single-sample variance")
	}
}

func TestBuild(t *testing.T) {
	clean := []model.Obs{
		{Variant: 0, Converted: 1, Revenue: 10},
		{Variant: 0, Converted: 0, Revenue: 0},
		{Variant: 1, Converted: 1, Revenue: 20},
	}
	d, err := Build(clean, []string{"converted", "revenue"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Acc[0][0].N != 2 || d.Acc[0][0].Mean != 0.5 || d.Acc[1][1].Mean != 20 {
		t.Errorf("unexpected accumulators: %+v", d.Acc)
	}
	if len(d.Samples[0][1]) != 2 || len(d.Samples[1][0]) != 1 {
		t.Errorf("unexpected samples: %+v", d.Samples)
	}
	if _, err := Build(clean, []string{"clicks"}); err == nil {
		t.Error("expected error for unknown metric")
	}
}
