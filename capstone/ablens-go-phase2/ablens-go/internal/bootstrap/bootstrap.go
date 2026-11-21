package bootstrap

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"sort"
	"sync"
	"time"
)

const chunkSize = 32

type Result struct {
	Iterations int
	Workers    int
	CILow      float64
	CIHigh     float64
	StdErr     float64
	Elapsed    time.Duration
}

type chunkResult struct {
	start int
	diffs []float64
}

func Run(ctx context.Context, control, treatment []float64, iterations, workers int, seed uint64, alpha float64) (Result, error) {
	if len(control) == 0 || len(treatment) == 0 {
		return Result{}, errors.New("bootstrap needs observations in both variants")
	}
	if iterations < 1 {
		return Result{}, errors.New("bootstrap needs at least one iteration")
	}
	if workers < 1 {
		workers = 1
	}
	began := time.Now()
	chunks := (iterations + chunkSize - 1) / chunkSize

	jobs := make(chan int)
	results := make(chan chunkResult, workers)

	go func() {
		defer close(jobs)
		for c := 0; c < chunks; c++ {
			select {
			case jobs <- c:
			case <-ctx.Done():
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				start := c * chunkSize
				end := min(start+chunkSize, iterations)
				rng := rand.New(rand.NewPCG(seed, uint64(c)))
				diffs := make([]float64, end-start)
				for i := range diffs {
					if ctx.Err() != nil {
						return
					}
					diffs[i] = resampleMean(rng, treatment) - resampleMean(rng, control)
				}
				select {
				case results <- chunkResult{start: start, diffs: diffs}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	all := make([]float64, iterations)
	got := 0
	for cr := range results {
		copy(all[cr.start:], cr.diffs)
		got += len(cr.diffs)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if got != iterations {
		return Result{}, errors.New("bootstrap finished with missing iterations")
	}

	var sum, sq float64
	for _, d := range all {
		sum += d
	}
	mean := sum / float64(iterations)
	for _, d := range all {
		sq += (d - mean) * (d - mean)
	}
	se := 0.0
	if iterations > 1 {
		se = math.Sqrt(sq / float64(iterations-1))
	}
	sort.Float64s(all)
	return Result{
		Iterations: iterations,
		Workers:    workers,
		CILow:      quantile(all, alpha/2),
		CIHigh:     quantile(all, 1-alpha/2),
		StdErr:     se,
		Elapsed:    time.Since(began),
	}, nil
}

func resampleMean(rng *rand.Rand, xs []float64) float64 {
	n := len(xs)
	var s float64
	for i := 0; i < n; i++ {
		s += xs[rng.IntN(n)]
	}
	return s / float64(n)
}

func quantile(sorted []float64, q float64) float64 {
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	return sorted[lo] + (pos-float64(lo))*(sorted[hi]-sorted[lo])
}
