# ABLens-Go

A concurrent A/B experiment analysis engine written in Go (ENSP465 Capstone, Phase II).

It reads an experiment event log (CSV or JSON Lines), validates it, runs significance tests, and computes bootstrap confidence intervals on a pool of goroutines. Output is a JSON report and a Markdown report with a SHIP / DO NOT SHIP / INCONCLUSIVE / INVALID DATA verdict.

Standard library only. No third-party modules.

## Build and run

```
go build -o ablens .

./ablens generate -out events.csv -rows 100000 -lift 0.01 -dup-rate 0.01 -cross-rate 0.005 -bad-rate 0.005
./ablens run -data events.csv -config examples/experiment.yaml -out report -v
```

A ready-made sample is in `examples/` (`sample_events.csv`, `experiment.yaml`, and the resulting `sample_report/`).

Exit codes: `0` ok, `1` error, `2` usage, `3` data failed validation (sample ratio mismatch).

`run` flags: `-data`, `-config`, `-out` (default `report`), `-workers`, `-iterations` (0 disables the bootstrap), `-seed`, `-v`.

## Input format

CSV (header required, any column order, extra columns ignored) or JSON Lines, chosen by file extension (`.csv`, `.jsonl`, `.json`, `.ndjson`).

| Field | Required | Notes |
|---|---|---|
| `event_id` | yes | unique per row; repeats are dropped as duplicates |
| `user_id` | yes | users seen in both variants are excluded entirely |
| `variant` | yes | must equal `control` or `treatment` from the config |
| `timestamp` | yes | RFC 3339 |
| `converted` | yes | 0 or 1 |
| `revenue` | no | number >= 0, blank means 0 |

Each valid row is treated as one independent observation. The tool assumes one row per user per experiment, which the cross-variant check helps enforce.

## Config (`experiment.yaml` or `.json`)

```
name: checkout-button-test
control: control
treatment: treatment
expected_split: [0.5, 0.5]
alpha: 0.05
srm_alpha: 0.001
bootstrap_iterations: 2000
workers: 4
seed: 42
metrics:
  - converted      # binary, two-proportion z-test
  - revenue        # continuous, Welch t-test
```

All keys are optional; defaults apply (50/50 split, alpha 0.05, 10,000 iterations, one worker per CPU, metric `converted`). The first metric is the primary metric used for the verdict. Unknown keys are rejected. The YAML reader supports the subset shown above (scalars, inline lists, block lists, comments).

## Architecture (maps to the Phase I modules)

| Module | Package |
|---|---|
| M1 CLI and config loader | `cmd`, `internal/config` |
| M2 Ingestion reader (streaming, `Reader` interface) | `internal/ingest` |
| M3 Validator and cleaner (+ SRM check) | `internal/validate` |
| M4 Aggregator (Welford single pass) | `internal/agg` |
| M5 Test engine (worker pool) | `internal/stats`, `internal/pipeline` |
| M6 Bootstrap engine (worker pool) | `internal/bootstrap` |
| M7 Result collector (fan-in) | `internal/bootstrap`, `internal/pipeline` |
| M8 Report generator (`Reporter` interface) | `internal/report` |
| M9 Logging | `log/slog` via `-v` |
| Synthetic data generator | `internal/gen` |

Concurrency:

- A reader goroutine streams batches of rows over a buffered channel to the validator.
- Test workers consume per-metric jobs from a channel and send results back over a results channel.
- Bootstrap iterations are split into chunks of 32. Workers pull chunks from a jobs channel and send results over a results channel. Each chunk has its own RNG seeded from `(seed, chunkIndex)`, so output is identical for any worker count.
- A `context.Context` cancels every stage on error or Ctrl-C. Bootstrap workers check it on every iteration.

## Data-quality checks

Malformed rows (bad timestamp, non 0/1 conversion, negative or non-finite revenue, missing ids, wrong field count, broken JSON), unknown variants, duplicate event ids, users in both variants, and sample ratio mismatch (chi-square goodness of fit against `expected_split`, flagged when p < `srm_alpha`). If SRM is flagged the verdict is INVALID DATA and the bootstrap is skipped.

## Tests and verification

```
go test -race ./...
python3 scripts/crosscheck.py events.csv report/report.json     # needs numpy and scipy
```

- Unit tests for every package; `go test -race` is clean; statement coverage is about 90% overall (about 94% or more on validate, stats and bootstrap).
- `mathx` (normal, Student-t and chi-square functions, written for this project) is tested against SciPy reference values.
- `scripts/crosscheck.py` independently re-cleans a raw CSV and recomputes every count and statistic with SciPy. On a 150,000-row dirty dataset all 22 checks agree (counts exactly, statistics to about 1e-12).

## Measured on this development machine (2 cores)

| Target | Result |
|---|---|
| O1: 1M rows through validation and aggregation in 5 s or less | 1.8 s (CSV), 3.9 s (JSONL), about 270 MB peak memory |
| O3: bootstrap speed-up with more workers | 1 to 2 workers: about 1.6x on 2 cores. The 3x on 4 cores target has not been measured yet (needs a 4-core machine) |
| O3: identical results for the same seed | verified for 1, 2, 3, 4 and 8 workers |

## Known limitations and Phase III work

- The bootstrap needs the per-variant observations in memory, so memory grows with the row count (the Phase I text described constant-memory aggregation for the summary statistics only).
- Bootstrap cost is rows x iterations. 10,000 resamples on 1M rows is slow; the example config uses 2,000.
- HTML report, benchmark write-up on a 4-core machine, and user guide are planned for Phase III.
- Cobra, yaml.v3 and Gonum from the Phase I plan are replaced by the standard library (`flag`, a small YAML-subset parser, and a custom `mathx` package), because third-party module downloads were not available in the build environment. They can be swapped back in behind the same interfaces.
