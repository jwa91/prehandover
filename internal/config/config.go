package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Manifest    Manifest `toml:"manifest"`
	Budget      Duration `toml:"budget"`
	Parallelism string   `toml:"parallelism"`
	OnUnchanged string   `toml:"on_unchanged"`
	FailFast    bool     `toml:"fail_fast"`
	Files       Pattern  `toml:"files"`
	Exclude     Pattern  `toml:"exclude"`
	Checks      []Check  `toml:"checks"`
}

type Manifest struct {
	Project             string   `toml:"project"`
	Moments             []string `toml:"moments"`
	Adapters            []string `toml:"adapters"`
	RequiredPrehandover string   `toml:"required_prehandover"`
}

type Check struct {
	ID            string            `toml:"id"`
	Name          string            `toml:"name"`
	Entry         string            `toml:"entry"`
	Args          []string          `toml:"args"`
	Files         Pattern           `toml:"files"`
	Exclude       Pattern           `toml:"exclude"`
	PassFilenames PassFilenames     `toml:"pass_filenames"`
	AlwaysRun     bool              `toml:"always_run"`
	RequireSerial bool              `toml:"require_serial"`
	Verbose       bool              `toml:"verbose"`
	Env           map[string]string `toml:"env"`
	Priority      int               `toml:"priority"`
	Budget        Duration          `toml:"budget"`
	Description   string            `toml:"description"`
	Shell         string            `toml:"shell"` // sh|bash — when set, entry is run via <shell> -c
}

// Duration parses Go duration strings like "500ms", "3s", "1m".
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		return nil
	}
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", text, err)
	}
	d.Duration = v
	return nil
}

// Pattern accepts either a regex string or an inline table {glob = "..."} / {glob = [...]} / {regex = "..."}.
type Pattern struct {
	Regex string
	Globs []string
}

func (p *Pattern) UnmarshalTOML(v interface{}) error {
	switch x := v.(type) {
	case string:
		p.Regex = x
	case map[string]interface{}:
		for k := range x {
			if k != "glob" && k != "regex" {
				return fmt.Errorf("pattern: unknown key %q (want \"glob\" or \"regex\")", k)
			}
		}
		if g, ok := x["glob"]; ok {
			switch gv := g.(type) {
			case string:
				p.Globs = []string{gv}
			case []interface{}:
				for i, item := range gv {
					s, ok := item.(string)
					if !ok {
						return fmt.Errorf("pattern: glob[%d] must be string, got %T", i, item)
					}
					p.Globs = append(p.Globs, s)
				}
			default:
				return fmt.Errorf("pattern: glob must be string or array of strings, got %T", g)
			}
		}
		if r, ok := x["regex"]; ok {
			s, ok := r.(string)
			if !ok {
				return fmt.Errorf("pattern: regex must be string, got %T", r)
			}
			p.Regex = s
		}
	default:
		return fmt.Errorf("pattern: must be string or inline table, got %T", v)
	}
	return nil
}

// PassFilenames is bool or int (limit per invocation).
// Default when absent: true (matches prek).
type PassFilenames struct {
	Set     bool
	Enabled bool
	Limit   int
}

func (p *PassFilenames) UnmarshalTOML(v interface{}) error {
	p.Set = true
	switch x := v.(type) {
	case bool:
		p.Enabled = x
	case int64:
		if x <= 0 {
			return fmt.Errorf("pass_filenames: limit must be a positive integer, got %d", x)
		}
		p.Enabled = true
		p.Limit = int(x)
	default:
		return fmt.Errorf("pass_filenames: must be bool or non-negative int, got %T", v)
	}
	return nil
}

// Effective returns whether to pass filenames, applying the "default true" rule.
func (p PassFilenames) Effective() bool {
	if !p.Set {
		return true
	}
	return p.Enabled
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Manifest.Project == "" {
		return nil, fmt.Errorf("parse %s: manifest.project is required", path)
	}
	if len(cfg.Manifest.Moments) == 0 {
		return nil, fmt.Errorf("parse %s: manifest.moments is required", path)
	}
	if len(cfg.Manifest.Adapters) == 0 {
		return nil, fmt.Errorf("parse %s: manifest.adapters is required", path)
	}
	if cfg.Manifest.RequiredPrehandover == "" {
		return nil, fmt.Errorf("parse %s: manifest.required_prehandover is required", path)
	}
	if cfg.Budget.Duration == 0 {
		cfg.Budget.Duration = 5 * time.Second
	}
	if cfg.Parallelism == "" {
		cfg.Parallelism = "auto"
	}
	if cfg.Parallelism != "auto" {
		n, err := strconv.Atoi(cfg.Parallelism)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("parse %s: parallelism must be \"auto\" or a positive integer, got %q", path, cfg.Parallelism)
		}
	}
	if cfg.OnUnchanged == "" {
		cfg.OnUnchanged = "skip"
	}
	for i := range cfg.Checks {
		if cfg.Checks[i].Budget.Duration == 0 {
			cfg.Checks[i].Budget.Duration = cfg.Budget.Duration
		}
		if cfg.Checks[i].Name == "" {
			cfg.Checks[i].Name = cfg.Checks[i].ID
		}
	}
	return &cfg, nil
}
