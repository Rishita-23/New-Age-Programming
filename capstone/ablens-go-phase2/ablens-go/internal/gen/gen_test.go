package gen

import (
	"bytes"
	"strings"
	"testing"
)

func TestDeterministic(t *testing.T) {
	o := Defaults()
	o.Rows = 500
	o.DupRate, o.CrossRate, o.BadRate = 0.05, 0.05, 0.05
	var a, b bytes.Buffer
	if err := Generate(&a, o); err != nil {
		t.Fatal(err)
	}
	if err := Generate(&b, o); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Error("same options must give identical output")
	}
	o.Seed++
	var c bytes.Buffer
	Generate(&c, o)
	if a.String() == c.String() {
		t.Error("different seeds must differ")
	}
}

func TestCSVShape(t *testing.T) {
	o := Defaults()
	o.Rows = 100
	var b bytes.Buffer
	if err := Generate(&b, o); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 101 || lines[0] != "event_id,user_id,variant,timestamp,converted,revenue" {
		t.Errorf("unexpected csv: %d lines, header %q", len(lines), lines[0])
	}
}

func TestJSONLShape(t *testing.T) {
	o := Defaults()
	o.Rows = 50
	o.Format = "jsonl"
	var b bytes.Buffer
	if err := Generate(&b, o); err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		if !strings.HasPrefix(l, "{") || !strings.HasSuffix(l, "}") {
			t.Fatalf("not a json object: %q", l)
		}
	}
}

func TestSplitAndRatesAreRespected(t *testing.T) {
	o := Defaults()
	o.Rows = 40000
	o.Split = 0.7
	var b bytes.Buffer
	if err := Generate(&b, o); err != nil {
		t.Fatal(err)
	}
	ctl := strings.Count(b.String(), ",control,")
	if frac := float64(ctl) / 40000; frac < 0.68 || frac > 0.72 {
		t.Errorf("control share %.3f, want ~0.70", frac)
	}
}

func TestValidation(t *testing.T) {
	bad := []func(*Options){
		func(o *Options) { o.Rows = 0 },
		func(o *Options) { o.Format = "xml" },
		func(o *Options) { o.BaseRate = 0.99; o.Lift = 0.5 },
		func(o *Options) { o.Split = 1 },
		func(o *Options) { o.DupRate, o.CrossRate, o.BadRate = 0.5, 0.4, 0.3 },
		func(o *Options) { o.BadRate = -0.1 },
	}
	for i, f := range bad {
		o := Defaults()
		f(&o)
		if err := Generate(&bytes.Buffer{}, o); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}
