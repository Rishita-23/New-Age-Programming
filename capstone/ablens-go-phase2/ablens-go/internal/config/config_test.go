package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func write(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadYAML(t *testing.T) {
	p := write(t, "e.yaml", `# demo
name: "checkout test"
control: control
treatment: variant_b   # inline comment
expected_split: [0.5, 0.5]
alpha: 0.01
bootstrap_iterations: 500
workers: 3
seed: 42
metrics:
  - converted
  - revenue
`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "checkout test" || c.Treatment != "variant_b" || c.Alpha != 0.01 ||
		c.BootstrapIterations != 500 || c.Workers != 3 || c.Seed != 42 {
		t.Errorf("unexpected config: %+v", c)
	}
	if !reflect.DeepEqual(c.Metrics, []string{"converted", "revenue"}) {
		t.Errorf("metrics: %v", c.Metrics)
	}
}

func TestLoadJSONAndDefaults(t *testing.T) {
	p := write(t, "e.json", `{"name":"x","metrics":["converted"]}`)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Control != "control" || c.Alpha != 0.05 || c.BootstrapIterations != 10000 {
		t.Errorf("defaults not applied: %+v", c)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := map[string]string{
		"unknown key":   "bogus: 1\n",
		"bad number":    "alpha: abc\n",
		"bad split":     "expected_split: [0.6, 0.6]\n",
		"bad metric":    "metrics: [clicks]\n",
		"same variants": "control: a\ntreatment: a\n",
		"bad alpha":     "alpha: 1.5\n",
		"zero workers":  "workers: 0\n",
	}
	for name, body := range cases {
		if _, err := Load(write(t, "e.yaml", body)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil || strings.Contains(err.Error(), "invalid") {
		t.Errorf("missing file should surface os error, got %v", err)
	}
}
