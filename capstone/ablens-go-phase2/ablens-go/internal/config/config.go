package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type Config struct {
	Name                string    `json:"name"`
	Control             string    `json:"control"`
	Treatment           string    `json:"treatment"`
	ExpectedSplit       []float64 `json:"expected_split"`
	Alpha               float64   `json:"alpha"`
	SRMAlpha            float64   `json:"srm_alpha"`
	BootstrapIterations int       `json:"bootstrap_iterations"`
	Workers             int       `json:"workers"`
	Seed                uint64    `json:"seed"`
	Metrics             []string  `json:"metrics"`
}

func Default() Config {
	return Config{
		Name:                "experiment",
		Control:             "control",
		Treatment:           "treatment",
		ExpectedSplit:       []float64{0.5, 0.5},
		Alpha:               0.05,
		SRMAlpha:            0.001,
		BootstrapIterations: 10000,
		Workers:             runtime.NumCPU(),
		Seed:                1,
		Metrics:             []string{"converted"},
	}
}

func MetricKind(name string) (string, error) {
	switch name {
	case "converted":
		return "binary", nil
	case "revenue":
		return "continuous", nil
	}
	return "", fmt.Errorf("unknown metric %q (supported: converted, revenue)", name)
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	if strings.EqualFold(filepath.Ext(path), ".json") {
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", path, err)
		}
	} else {
		m, err := parseYAML(string(data))
		if err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", path, err)
		}
		if err := apply(&cfg, m); err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return cfg, nil
}

func apply(cfg *Config, m map[string]any) error {
	for k, v := range m {
		var err error
		switch k {
		case "name":
			cfg.Name, err = str(k, v)
		case "control":
			cfg.Control, err = str(k, v)
		case "treatment":
			cfg.Treatment, err = str(k, v)
		case "expected_split":
			cfg.ExpectedSplit, err = floats(k, v)
		case "alpha":
			cfg.Alpha, err = float(k, v)
		case "srm_alpha":
			cfg.SRMAlpha, err = float(k, v)
		case "bootstrap_iterations":
			cfg.BootstrapIterations, err = integer(k, v)
		case "workers":
			cfg.Workers, err = integer(k, v)
		case "seed":
			var s int
			s, err = integer(k, v)
			cfg.Seed = uint64(s)
		case "metrics":
			l, ok := v.([]string)
			if !ok {
				err = fmt.Errorf("%s must be a list", k)
			}
			cfg.Metrics = l
		default:
			err = fmt.Errorf("unknown key %q", k)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func str(k string, v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a single value", k)
	}
	return s, nil
}

func float(k string, v any) (float64, error) {
	s, err := str(k, v)
	if err != nil {
		return 0, err
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a number", k, s)
	}
	return f, nil
}

func integer(k string, v any) (int, error) {
	s, err := str(k, v)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not an integer", k, s)
	}
	return n, nil
}

func floats(k string, v any) ([]float64, error) {
	l, ok := v.([]string)
	if !ok {
		return nil, fmt.Errorf("%s must be a list", k)
	}
	out := make([]float64, len(l))
	for i, s := range l {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a number", k, s)
		}
		out[i] = f
	}
	return out, nil
}

func (c *Config) Validate() error {
	var errs []error
	if c.Control == "" || c.Treatment == "" {
		errs = append(errs, errors.New("control and treatment must be set"))
	}
	if c.Control == c.Treatment {
		errs = append(errs, errors.New("control and treatment must differ"))
	}
	if len(c.ExpectedSplit) != 2 {
		errs = append(errs, errors.New("expected_split must have exactly two values"))
	} else {
		a, b := c.ExpectedSplit[0], c.ExpectedSplit[1]
		if a <= 0 || b <= 0 || math.Abs(a+b-1) > 1e-9 {
			errs = append(errs, errors.New("expected_split values must be positive and sum to 1"))
		}
	}
	if !(c.Alpha > 0 && c.Alpha < 1) {
		errs = append(errs, errors.New("alpha must be between 0 and 1"))
	}
	if !(c.SRMAlpha > 0 && c.SRMAlpha < 1) {
		errs = append(errs, errors.New("srm_alpha must be between 0 and 1"))
	}
	if c.BootstrapIterations < 0 {
		errs = append(errs, errors.New("bootstrap_iterations must be >= 0"))
	}
	if c.Workers < 1 {
		errs = append(errs, errors.New("workers must be >= 1"))
	}
	if len(c.Metrics) == 0 {
		errs = append(errs, errors.New("at least one metric is required"))
	}
	seen := map[string]bool{}
	for _, m := range c.Metrics {
		if _, err := MetricKind(m); err != nil {
			errs = append(errs, err)
		}
		if seen[m] {
			errs = append(errs, fmt.Errorf("duplicate metric %q", m))
		}
		seen[m] = true
	}
	return errors.Join(errs...)
}
