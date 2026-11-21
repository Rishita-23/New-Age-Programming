package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ablens/internal/result"
)

type Reporter interface {
	Name() string
	Write(dir string, res *result.Result) (string, error)
}

func All() []Reporter { return []Reporter{JSON{}, Markdown{}} }

func WriteAll(dir string, res *result.Result) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var paths []string
	for _, r := range All() {
		p, err := r.Write(dir, res)
		if err != nil {
			return paths, fmt.Errorf("%s report: %w", r.Name(), err)
		}
		paths = append(paths, p)
	}
	return paths, nil
}

type JSON struct{}

func (JSON) Name() string { return "json" }

func (JSON) Write(dir string, res *result.Result) (string, error) {
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "report.json")
	return p, os.WriteFile(p, append(data, '\n'), 0o644)
}

type Markdown struct{}

func (Markdown) Name() string { return "markdown" }

func (Markdown) Write(dir string, res *result.Result) (string, error) {
	p := filepath.Join(dir, "report.md")
	return p, os.WriteFile(p, []byte(RenderMarkdown(res)), 0o644)
}

func pct(x float64) string { return fmt.Sprintf("%.2f%%", x*100) }

func RenderMarkdown(res *result.Result) string {
	var b strings.Builder
	q := res.Quality
	fmt.Fprintf(&b, "# A/B Experiment Report: %s\n\n", res.Experiment)
	fmt.Fprintf(&b, "**Verdict: %s**\n\n%s.\n\n", res.Verdict, res.VerdictReason)

	b.WriteString("## Data quality\n\n| Check | Result |\n|---|---|\n")
	fmt.Fprintf(&b, "| Rows read | %d |\n| Valid rows analysed | %d |\n", q.TotalRows, q.ValidRows)
	fmt.Fprintf(&b, "| Malformed rows | %d |\n| Unknown-variant rows | %d |\n", q.MalformedRows, q.UnknownVariant)
	fmt.Fprintf(&b, "| Duplicate event rows removed | %d |\n", q.DuplicateEvents)
	fmt.Fprintf(&b, "| Users seen in both variants (excluded) | %d (%d rows) |\n", q.CrossVariantUsers, q.CrossVariantRows)
	status := "pass"
	if q.SRM.Flagged {
		status = "FAIL"
	}
	fmt.Fprintf(&b, "| Sample ratio check | %s: observed %d / %d, expected %.0f / %.0f, chi-square %.3f, p = %.3g |\n\n",
		status, q.SRM.Observed[0], q.SRM.Observed[1], q.SRM.Expected[0], q.SRM.Expected[1], q.SRM.ChiSquare, q.SRM.PValue)
	if len(q.MalformedReasons) > 0 {
		keys := make([]string, 0, len(q.MalformedReasons))
		for k := range q.MalformedReasons {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("Malformed row reasons:\n\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "- %s: %d\n", k, q.MalformedReasons[k])
		}
		b.WriteString("\n")
	}

	b.WriteString("## Results\n\n")
	for _, m := range res.Metrics {
		fmt.Fprintf(&b, "### %s (%s)\n\n", m.Metric, m.Kind)
		b.WriteString("| Variant | n | Mean |\n|---|---|---|\n")
		fmt.Fprintf(&b, "| %s | %d | %.6g |\n| %s | %d | %.6g |\n\n", m.Control.Name, m.Control.N, m.Control.Mean, m.Treatment.Name, m.Treatment.N, m.Treatment.Mean)
		fmt.Fprintf(&b, "- Absolute lift: %.6g", m.AbsLift)
		if m.RelLift != nil {
			fmt.Fprintf(&b, " (relative %s)", pct(*m.RelLift))
		}
		b.WriteString("\n")
		if m.Test != nil {
			t := m.Test
			fmt.Fprintf(&b, "- %s: statistic %.4f, p = %.4g (%s at alpha %.3g)\n", t.Name, t.Statistic, t.PValue, sig(t.Significant), res.Alpha)
			fmt.Fprintf(&b, "- Analytic %.0f%% CI for the difference: [%.6g, %.6g]\n", (1-res.Alpha)*100, t.CILow, t.CIHigh)
		} else {
			fmt.Fprintf(&b, "- Test not run: %s\n", m.TestError)
		}
		if m.Bootstrap != nil {
			bs := m.Bootstrap
			fmt.Fprintf(&b, "- Bootstrap %.0f%% CI (%d resamples, %d workers): [%.6g, %.6g]\n", (1-res.Alpha)*100, bs.Iterations, bs.Workers, bs.CILow, bs.CIHigh)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Timings (seconds)\n\n")
	keys := make([]string, 0, len(res.Timings))
	for k := range res.Timings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "- %s: %.3f\n", k, res.Timings[k])
	}
	return b.String()
}

func sig(s bool) string {
	if s {
		return "significant"
	}
	return "not significant"
}
