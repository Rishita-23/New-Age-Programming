package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ablens/internal/config"
	"ablens/internal/gen"
)

func dataset(t *testing.T, format string, mod func(*gen.Options)) string {
	t.Helper()
	o := gen.Defaults()
	o.Rows = 30000
	o.Format = format
	o.Seed = 11
	if mod != nil {
		mod(&o)
	}
	p := filepath.Join(t.TempDir(), "events."+format)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := gen.Generate(f, o); err != nil {
		t.Fatal(err)
	}
	return p
}

func cfgFor(metrics ...string) config.Config {
	c := config.Default()
	c.BootstrapIterations = 200
	c.Workers = 4
	c.Metrics = metrics
	return c
}

func TestDetectsRealLiftCSVAndJSONL(t *testing.T) {
	for _, format := range []string{"csv", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			p := dataset(t, format, func(o *gen.Options) { o.Lift = 0.03 })
			res, err := Run(context.Background(), cfgFor("converted", "revenue"), p, nil)
			if err != nil {
				t.Fatal(err)
			}
			if res.Verdict != VerdictShip {
				t.Errorf("verdict %s: %s", res.Verdict, res.VerdictReason)
			}
			m := res.Metrics[0]
			if m.Test == nil || !m.Test.Significant || m.AbsLift < 0.01 || m.AbsLift > 0.05 {
				t.Errorf("converted: %+v", m)
			}
			if m.Bootstrap == nil || m.Bootstrap.CILow > m.AbsLift || m.Bootstrap.CIHigh < m.AbsLift {
				t.Errorf("bootstrap CI should bracket the observed lift: %+v lift=%v", m.Bootstrap, m.AbsLift)
			}
			if res.Metrics[1].Test == nil || res.Metrics[1].Test.Name != "Welch t-test" {
				t.Errorf("revenue should use Welch: %+v", res.Metrics[1])
			}
			if res.Quality.SRM.Flagged {
				t.Errorf("unexpected SRM flag: %+v", res.Quality.SRM)
			}
		})
	}
}

func TestNoEffectIsInconclusive(t *testing.T) {
	p := dataset(t, "csv", func(o *gen.Options) { o.Lift = 0; o.RevenueLift = 0 })
	res, err := Run(context.Background(), cfgFor("converted"), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != VerdictInconclusive {
		t.Errorf("verdict %s: %s", res.Verdict, res.VerdictReason)
	}
}

func TestNegativeLiftDoNotShip(t *testing.T) {
	p := dataset(t, "csv", func(o *gen.Options) { o.Lift = -0.04 })
	res, err := Run(context.Background(), cfgFor("converted"), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != VerdictDoNotShip {
		t.Errorf("verdict %s: %s", res.Verdict, res.VerdictReason)
	}
}

func TestSRMInvalidatesAndSkipsBootstrap(t *testing.T) {
	p := dataset(t, "csv", func(o *gen.Options) { o.Split = 0.6 })
	res, err := Run(context.Background(), cfgFor("converted"), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != VerdictInvalid || !res.Quality.SRM.Flagged {
		t.Errorf("verdict %s srm %+v", res.Verdict, res.Quality.SRM)
	}
	if res.Metrics[0].Bootstrap != nil {
		t.Error("bootstrap should be skipped for invalid data")
	}
}

func TestDirtyDataIsCountedAndCleaned(t *testing.T) {
	p := dataset(t, "csv", func(o *gen.Options) { o.DupRate = 0.02; o.CrossRate = 0.01; o.BadRate = 0.02 })
	res, err := Run(context.Background(), cfgFor("converted"), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	q := res.Quality
	if q.DuplicateEvents == 0 || q.CrossVariantUsers == 0 || q.MalformedRows == 0 || q.UnknownVariant == 0 {
		t.Errorf("expected every issue to be detected: %+v", q)
	}
	if q.ValidRows+q.MalformedRows+q.UnknownVariant+q.DuplicateEvents+q.CrossVariantRows != q.TotalRows {
		t.Errorf("row accounting does not add up: %+v", q)
	}
}

func TestSameSeedSameResultRegardlessOfWorkers(t *testing.T) {
	p := dataset(t, "csv", nil)
	var first []byte
	for _, w := range []int{1, 3, 8} {
		c := cfgFor("converted", "revenue")
		c.Workers = w
		res, err := Run(context.Background(), c, p, nil)
		if err != nil {
			t.Fatal(err)
		}
		for i := range res.Metrics {
			res.Metrics[i].Bootstrap.Workers = 0
			res.Metrics[i].Bootstrap.Seconds = 0
		}
		res.Timings = nil
		b, _ := json.Marshal(res)
		if first == nil {
			first = b
		} else if string(first) != string(b) {
			t.Errorf("workers=%d changed the result", w)
		}
	}
}

func TestErrors(t *testing.T) {
	if _, err := Run(context.Background(), cfgFor("converted"), "missing.csv", nil); err == nil {
		t.Error("missing file")
	}
	bad := filepath.Join(t.TempDir(), "e.csv")
	os.WriteFile(bad, []byte("a,b\n1,2\n"), 0o600)
	if _, err := Run(context.Background(), cfgFor("converted"), bad, nil); err == nil || !strings.Contains(err.Error(), "missing required column") {
		t.Errorf("bad header: %v", err)
	}
	c := cfgFor("converted")
	c.Alpha = 2
	if _, err := Run(context.Background(), c, bad, nil); err == nil {
		t.Error("invalid config")
	}
	allBad := filepath.Join(t.TempDir(), "e2.csv")
	os.WriteFile(allBad, []byte("event_id,user_id,variant,timestamp,converted\nx,y,control,bad,1\n"), 0o600)
	if _, err := Run(context.Background(), cfgFor("converted"), allBad, nil); err == nil {
		t.Error("no valid rows should error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := dataset(t, "csv", nil)
	if _, err := Run(ctx, cfgFor("converted"), p, nil); err == nil {
		t.Error("cancelled context should error")
	}
}

func TestZeroVarianceMetricReportsTestError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "e.csv")
	var sb strings.Builder
	sb.WriteString("event_id,user_id,variant,timestamp,converted\n")
	for i := 0; i < 50; i++ {
		v := "control"
		if i%2 == 1 {
			v = "treatment"
		}
		sb.WriteString("e" + string(rune('A'+i%26)) + string(rune('a'+i/26)) + ",u" + string(rune('A'+i%26)) + string(rune('a'+i/26)) + "," + v + ",2026-10-01T00:00:00Z,0\n")
	}
	os.WriteFile(p, []byte(sb.String()), 0o600)
	c := cfgFor("converted")
	c.BootstrapIterations = 0
	res, err := Run(context.Background(), c, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Metrics[0].Test != nil || res.Metrics[0].TestError == "" || res.Verdict != VerdictInconclusive {
		t.Errorf("expected untestable metric, got %+v verdict=%s", res.Metrics[0], res.Verdict)
	}
}
