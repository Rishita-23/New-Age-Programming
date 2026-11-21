package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := Execute(args, &out, &errb)
	return code, out.String(), errb.String()
}

func writeConfig(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, "exp.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const baseCfg = "name: t\nbootstrap_iterations: 100\nworkers: 2\nmetrics: [converted, revenue]\n"

func TestGenerateThenRun(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "events.csv")
	if code, _, e := run("generate", "-out", data, "-rows", "20000", "-seed", "3", "-lift", "0.03"); code != ExitOK {
		t.Fatalf("generate: %d %s", code, e)
	}
	cfg := writeConfig(t, dir, baseCfg)
	out := filepath.Join(dir, "rep")
	code, stdout, e := run("run", "-data", data, "-config", cfg, "-out", out)
	if code != ExitOK {
		t.Fatalf("run: %d %s", code, e)
	}
	for _, want := range []string{"Verdict: SHIP", "converted", "revenue", "report.json", "report.md"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("summary missing %q:\n%s", want, stdout)
		}
	}
	for _, f := range []string{"report.json", "report.md"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Error(err)
		}
	}
}

func TestSRMExitCode(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "events.jsonl")
	if code, _, e := run("generate", "-out", data, "-format", "jsonl", "-rows", "20000", "-split", "0.65"); code != ExitOK {
		t.Fatalf("generate: %d %s", code, e)
	}
	code, stdout, _ := run("run", "-data", data, "-config", writeConfig(t, dir, baseCfg), "-out", filepath.Join(dir, "r"))
	if code != ExitInvalidData || !strings.Contains(stdout, "INVALID DATA") {
		t.Errorf("code=%d stdout=%s", code, stdout)
	}
}

func TestOverrides(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "events.csv")
	run("generate", "-out", data, "-rows", "5000")
	code, stdout, e := run("run", "-data", data, "-config", writeConfig(t, dir, baseCfg), "-out", filepath.Join(dir, "r"),
		"-iterations", "0", "-workers", "1", "-seed", "9")
	if code != ExitOK {
		t.Fatalf("%d %s", code, e)
	}
	if strings.Contains(stdout, "bootstrap CI") {
		t.Error("-iterations 0 should disable bootstrap")
	}
}

func TestUsageAndErrors(t *testing.T) {
	if code, _, _ := run(); code != ExitUsage {
		t.Error("no args")
	}
	if code, out, _ := run("help"); code != ExitOK || !strings.Contains(out, "Usage") {
		t.Error("help")
	}
	if code, _, e := run("bogus"); code != ExitUsage || !strings.Contains(e, "unknown command") {
		t.Error("unknown command")
	}
	if code, _, _ := run("run"); code != ExitUsage {
		t.Error("run without flags")
	}
	if code, _, _ := run("run", "-nope"); code != ExitUsage {
		t.Error("bad flag")
	}
	if code, _, _ := run("generate"); code != ExitUsage {
		t.Error("generate without -out")
	}
	dir := t.TempDir()
	if code, _, e := run("run", "-data", "missing.csv", "-config", filepath.Join(dir, "nope.yaml")); code != ExitError || e == "" {
		t.Error("missing config")
	}
	cfg := writeConfig(t, dir, baseCfg)
	if code, _, e := run("run", "-data", filepath.Join(dir, "missing.csv"), "-config", cfg); code != ExitError || !strings.Contains(e, "error:") {
		t.Errorf("missing data: %d %s", code, e)
	}
	if code, _, _ := run("run", "-data", "x.csv", "-config", cfg, "-workers", "-5", "-iterations", "-1"); code == ExitOK {
		t.Error("expected failure on missing data file")
	}
	if code, _, e := run("generate", "-out", filepath.Join(dir, "x.csv"), "-rows", "0"); code != ExitError || e == "" {
		t.Error("invalid generate options")
	}
	if code, _, _ := run("generate", "-out", filepath.Join(dir, "no", "such", "dir.csv")); code != ExitError {
		t.Error("unwritable output")
	}
}
