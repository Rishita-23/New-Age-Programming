package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"ablens/internal/agg"
	"ablens/internal/bootstrap"
	"ablens/internal/config"
	"ablens/internal/ingest"
	"ablens/internal/model"
	"ablens/internal/result"
	"ablens/internal/stats"
	"ablens/internal/validate"
)

const (
	VerdictShip         = "SHIP"
	VerdictDoNotShip    = "DO NOT SHIP"
	VerdictInconclusive = "INCONCLUSIVE"
	VerdictInvalid      = "INVALID DATA"
)

func Run(ctx context.Context, cfg config.Config, dataPath string, log *slog.Logger) (*result.Result, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	timings := map[string]float64{}
	began := time.Now()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	clean, quality, err := ingestAndValidate(ctx, cfg, dataPath)
	timings["ingest_validate"] = time.Since(began).Seconds()
	if err != nil {
		return nil, err
	}
	log.Info("ingest complete", "rows", quality.TotalRows, "valid", quality.ValidRows, "seconds", timings["ingest_validate"])

	t0 := time.Now()
	ds, err := agg.Build(clean, cfg.Metrics)
	if err != nil {
		return nil, err
	}
	clean = nil
	timings["aggregate"] = time.Since(t0).Seconds()

	t0 = time.Now()
	metrics, err := runTests(ctx, cfg, ds)
	if err != nil {
		return nil, err
	}
	timings["tests"] = time.Since(t0).Seconds()

	t0 = time.Now()
	if cfg.BootstrapIterations > 0 && !quality.SRM.Flagged {
		for i := range metrics {
			res, err := bootstrap.Run(ctx, ds.Samples[0][i], ds.Samples[1][i], cfg.BootstrapIterations, cfg.Workers, cfg.Seed, cfg.Alpha)
			if err != nil {
				return nil, fmt.Errorf("bootstrap %s: %w", metrics[i].Metric, err)
			}
			metrics[i].Bootstrap = &result.Bootstrap{
				Iterations: res.Iterations, Workers: res.Workers, CILow: res.CILow,
				CIHigh: res.CIHigh, StdErr: res.StdErr, Seconds: res.Elapsed.Seconds(),
			}
			log.Info("bootstrap complete", "metric", metrics[i].Metric, "iterations", res.Iterations, "workers", res.Workers, "seconds", res.Elapsed.Seconds())
		}
	}
	timings["bootstrap"] = time.Since(t0).Seconds()
	timings["total"] = time.Since(began).Seconds()

	verdict, reason := decide(cfg, quality, metrics)
	return &result.Result{
		Experiment: cfg.Name, Alpha: cfg.Alpha, Quality: quality, Metrics: metrics,
		Verdict: verdict, VerdictReason: reason, Timings: timings,
	}, nil
}

func ingestAndValidate(ctx context.Context, cfg config.Config, path string) ([]model.Obs, result.Quality, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rows := make(chan []model.Row, 8)
	readErr := make(chan error, 1)
	go func() {
		defer close(rows)
		readErr <- ingest.ReadFile(ctx, path, rows)
	}()
	clean, q, vErr := validate.Run(ctx, rows, cfg)
	if vErr != nil {
		cancel()
		for range rows {
		}
	}
	rErr := <-readErr
	if rErr != nil {
		return nil, q, rErr
	}
	return clean, q, vErr
}

type testJob struct {
	idx int
	ds  *agg.Dataset
	cfg config.Config
}

func runTests(ctx context.Context, cfg config.Config, ds *agg.Dataset) ([]result.MetricResult, error) {
	out := make([]result.MetricResult, len(cfg.Metrics))
	jobs := make(chan testJob)
	type done struct {
		idx int
		res result.MetricResult
	}
	results := make(chan done, len(cfg.Metrics))

	workers := min(cfg.Workers, len(cfg.Metrics))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				results <- done{j.idx, testMetric(j.cfg, j.ds, j.idx)}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range cfg.Metrics {
			select {
			case jobs <- testJob{i, ds, cfg}:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	n := 0
	for d := range results {
		out[d.idx] = d.res
		n++
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if n != len(cfg.Metrics) {
		return nil, fmt.Errorf("only %d of %d metrics were tested", n, len(cfg.Metrics))
	}
	return out, nil
}

func testMetric(cfg config.Config, ds *agg.Dataset, i int) result.MetricResult {
	name := cfg.Metrics[i]
	kind, _ := config.MetricKind(name)
	c, t := ds.Acc[0][i], ds.Acc[1][i]
	mr := result.MetricResult{
		Metric: name, Kind: kind,
		Control:   result.VariantSummary{Name: cfg.Control, N: c.N, Mean: c.Mean, Variance: c.Variance()},
		Treatment: result.VariantSummary{Name: cfg.Treatment, N: t.N, Mean: t.Mean, Variance: t.Variance()},
		AbsLift:   t.Mean - c.Mean,
	}
	if c.Mean != 0 {
		rel := (t.Mean - c.Mean) / c.Mean
		mr.RelLift = &rel
	}
	var tr stats.TestResult
	var err error
	if kind == "binary" {
		tr, err = stats.TwoProportionZ(c.N, c.Mean*float64(c.N), t.N, t.Mean*float64(t.N), cfg.Alpha)
	} else {
		tr, err = stats.Welch(c.N, c.Mean, c.Variance(), t.N, t.Mean, t.Variance(), cfg.Alpha)
	}
	if err != nil {
		mr.TestError = err.Error()
	} else {
		mr.Test = &tr
	}
	return mr
}

func decide(cfg config.Config, q result.Quality, metrics []result.MetricResult) (string, string) {
	if q.SRM.Flagged {
		return VerdictInvalid, fmt.Sprintf("sample ratio mismatch (p=%.3g < %.3g): the traffic split differs from the expected %.0f/%.0f, so results are not trustworthy",
			q.SRM.PValue, cfg.SRMAlpha, cfg.ExpectedSplit[0]*100, cfg.ExpectedSplit[1]*100)
	}
	primary := metrics[0]
	if primary.Test == nil {
		return VerdictInconclusive, fmt.Sprintf("primary metric %q could not be tested: %s", primary.Metric, primary.TestError)
	}
	switch {
	case !primary.Test.Significant:
		return VerdictInconclusive, fmt.Sprintf("no significant change in %q (p=%.3g, alpha=%.3g)", primary.Metric, primary.Test.PValue, cfg.Alpha)
	case primary.AbsLift > 0:
		return VerdictShip, fmt.Sprintf("%q improved significantly (p=%.3g)", primary.Metric, primary.Test.PValue)
	case primary.AbsLift < 0 && !math.IsNaN(primary.AbsLift):
		return VerdictDoNotShip, fmt.Sprintf("%q decreased significantly (p=%.3g)", primary.Metric, primary.Test.PValue)
	}
	return VerdictInconclusive, "no measurable difference"
}
