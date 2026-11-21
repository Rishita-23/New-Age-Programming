# A/B Experiment Report: checkout-button-test

**Verdict: SHIP**

"converted" improved significantly (p=0.024).

## Data quality

| Check | Result |
|---|---|
| Rows read | 20426 |
| Valid rows analysed | 19904 |
| Malformed rows | 88 |
| Unknown-variant rows | 27 |
| Duplicate event rows removed | 215 |
| Users seen in both variants (excluded) | 96 (192 rows) |
| Sample ratio check | pass: observed 9866 / 10038, expected 9952 / 9952, chi-square 1.486, p = 0.223 |

Malformed row reasons:

- invalid converted: 28
- invalid revenue: 32
- invalid timestamp: 28

## Results

### converted (binary)

| Variant | n | Mean |
|---|---|---|
| control | 9866 | 0.100649 |
| treatment | 10038 | 0.11048 |

- Absolute lift: 0.00983148 (relative 9.77%)
- two-proportion z-test: statistic 2.2565, p = 0.02404 (significant at alpha 0.05)
- Analytic 95% CI for the difference: [0.00129607, 0.0183669]
- Bootstrap 95% CI (2000 resamples, 2 workers): [0.00118218, 0.0184997]

### revenue (continuous)

| Variant | n | Mean |
|---|---|---|
| control | 9866 | 1.9644 |
| treatment | 10038 | 2.24217 |

- Absolute lift: 0.277767 (relative 14.14%)
- Welch t-test: statistic 2.1772, p = 0.02948 (significant at alpha 0.05)
- Analytic 95% CI for the difference: [0.0276986, 0.527835]
- Bootstrap 95% CI (2000 resamples, 2 workers): [0.0114967, 0.525922]

## Timings (seconds)

- aggregate: 0.001
- bootstrap: 0.237
- ingest_validate: 0.023
- tests: 0.000
- total: 0.261
