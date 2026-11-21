package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"

	"ablens/internal/config"
	"ablens/internal/gen"
	"ablens/internal/pipeline"
	"ablens/internal/report"
	"ablens/internal/result"
)

const (
	ExitOK          = 0
	ExitError       = 1
	ExitUsage       = 2
	ExitInvalidData = 3
)

const usage = `ablens - concurrent A/B experiment analysis

Usage:
  ablens run      -data <events.csv|events.jsonl> -config <experiment.yaml> [-out dir] [-workers N] [-iterations N] [-seed N] [-v]
  ablens generate -out <file> [-rows N] [-format csv|jsonl] [-seed N] [-base-rate F] [-lift F] [-split F]
                  [-mean-revenue F] [-revenue-lift F] [-dup-rate F] [-cross-rate F] [-bad-rate F]

Exit codes: 0 ok, 1 error, 2 usage, 3 data failed validation (sample ratio mismatch)
`

func Execute(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	switch args[0] {
	case "run":
		return runCmd(args[1:], stdout, stderr)
	case "generate":
		return generateCmd(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return ExitOK
	}
	fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
	return ExitUsage
}

func runCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	data := fs.String("data", "", "event log (.csv or .jsonl)")
	cfgPath := fs.String("config", "", "experiment config (.yaml or .json)")
	out := fs.String("out", "report", "output directory")
	workers := fs.Int("workers", 0, "override worker count")
	iters := fs.Int("iterations", -1, "override bootstrap iterations (0 disables)")
	seed := fs.Int64("seed", -1, "override random seed")
	verbose := fs.Bool("v", false, "log progress to stderr")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *data == "" || *cfgPath == "" {
		fmt.Fprintln(stderr, "run requires -data and -config")
		return ExitUsage
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}
	if *workers > 0 {
		cfg.Workers = *workers
	}
	if *iters >= 0 {
		cfg.BootstrapIterations = *iters
	}
	if *seed >= 0 {
		cfg.Seed = uint64(*seed)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}

	var logger *slog.Logger
	if *verbose {
		logger = slog.New(slog.NewTextHandler(stderr, nil))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	res, err := pipeline.Run(ctx, cfg, *data, logger)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(stderr, "interrupted")
		} else {
			fmt.Fprintln(stderr, "error:", err)
		}
		return ExitError
	}
	paths, err := report.WriteAll(*out, res)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}
	summarize(stdout, res, paths)
	if res.Verdict == pipeline.VerdictInvalid {
		return ExitInvalidData
	}
	return ExitOK
}

func summarize(w io.Writer, res *result.Result, paths []string) {
	q := res.Quality
	fmt.Fprintf(w, "Experiment: %s\n", res.Experiment)
	fmt.Fprintf(w, "Rows: %d read, %d valid (%d malformed, %d duplicate, %d cross-variant users)\n",
		q.TotalRows, q.ValidRows, q.MalformedRows, q.DuplicateEvents, q.CrossVariantUsers)
	fmt.Fprintf(w, "SRM: observed %d/%d, p=%.3g, flagged=%v\n", q.SRM.Observed[0], q.SRM.Observed[1], q.SRM.PValue, q.SRM.Flagged)
	for _, m := range res.Metrics {
		fmt.Fprintf(w, "- %s: control %.5g -> treatment %.5g (lift %+.5g)", m.Metric, m.Control.Mean, m.Treatment.Mean, m.AbsLift)
		if m.Test != nil {
			fmt.Fprintf(w, ", p=%.4g", m.Test.PValue)
		}
		if m.Bootstrap != nil {
			fmt.Fprintf(w, ", bootstrap CI [%.5g, %.5g]", m.Bootstrap.CILow, m.Bootstrap.CIHigh)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "Verdict: %s - %s\n", res.Verdict, res.VerdictReason)
	fmt.Fprintf(w, "Total time: %.2fs\n", res.Timings["total"])
	fmt.Fprintf(w, "Reports: %s\n", strings.Join(paths, ", "))
}

func generateCmd(args []string, stdout, stderr io.Writer) int {
	o := gen.Defaults()
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "output file")
	fs.IntVar(&o.Rows, "rows", o.Rows, "number of rows")
	fs.StringVar(&o.Format, "format", o.Format, "csv or jsonl")
	seed := fs.Int64("seed", int64(o.Seed), "random seed")
	fs.Float64Var(&o.BaseRate, "base-rate", o.BaseRate, "control conversion rate")
	fs.Float64Var(&o.Lift, "lift", o.Lift, "absolute conversion lift for treatment")
	fs.Float64Var(&o.Split, "split", o.Split, "share of traffic sent to control")
	fs.Float64Var(&o.MeanRevenue, "mean-revenue", o.MeanRevenue, "mean revenue per converting user (control)")
	fs.Float64Var(&o.RevenueLift, "revenue-lift", o.RevenueLift, "relative revenue lift for treatment")
	fs.Float64Var(&o.DupRate, "dup-rate", 0, "share of rows duplicated")
	fs.Float64Var(&o.CrossRate, "cross-rate", 0, "share of users also logged in the other variant")
	fs.Float64Var(&o.BadRate, "bad-rate", 0, "share of corrupted rows added")
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	if *out == "" || *seed < 0 {
		fmt.Fprintln(stderr, "generate requires -out and a non-negative -seed")
		return ExitUsage
	}
	o.Seed = uint64(*seed)
	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}
	if err := gen.Generate(f, o); err != nil {
		f.Close()
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return ExitError
	}
	fmt.Fprintf(stdout, "wrote %d base rows to %s\n", o.Rows, *out)
	return ExitOK
}
