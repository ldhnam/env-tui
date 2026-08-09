// Package schema loads an optional .envigator.yaml and validates environment
// files against variable constraints (required, type, enum, default, secret).
package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Schema struct {
	Environments map[string]EnvDef `yaml:"environments"`
	Variables    map[string]VarDef `yaml:"variables"`
}

type EnvDef struct {
	File string `yaml:"file"`
}

type VarDef struct {
	Required bool     `yaml:"required"`
	Secret   bool     `yaml:"secret"`
	Type     string   `yaml:"type"`
	Default  string   `yaml:"default"`
	Enum     []string `yaml:"enum"`
}

// Result is a single variable validation outcome.
type Result struct {
	Name    string
	OK      bool
	Detail  string // failure detail (expected / actual lines)
	Secret  bool
	Default string
}

// Load reads .envigator.yaml from dir (or a sibling .envigator.yml).
func Load(dir string) (*Schema, error) {
	for _, name := range []string{".envigator.yaml", ".envigator.yml"} {
		p := filepath.Join(dir, name)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var s Schema
		if err := yaml.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return &s, nil
	}
	return nil, fmt.Errorf("no .envigator.yaml found in %s", dir)
}

// PickEnvironment resolves which environment to validate: name wins if given,
// otherwise "local", otherwise the environment whose file matches dir's
// primary .env, otherwise the first defined environment.
func (s *Schema) PickEnvironment(name string, primary string) string {
	if name != "" {
		return name
	}
	if _, ok := s.Environments["local"]; ok {
		return "local"
	}
	if primary != "" {
		for n, def := range s.Environments {
			if def.File == filepath.Base(primary) {
				return n
			}
		}
	}
	for n := range s.Environments {
		return n
	}
	return ""
}

// File returns the env file path for an environment ("" if none defined).
func (s *Schema) File(dir, env string) string {
	if def, ok := s.Environments[env]; ok {
		return filepath.Join(dir, def.File)
	}
	return ""
}

// Validate checks env values against every declared variable.
func (s *Schema) Validate(env map[string]string) []Result {
	keys := make([]string, 0, len(s.Variables))
	for k := range s.Variables {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []Result
	for _, k := range keys {
		def := s.Variables[k]
		val := env[k]
		r := Result{Name: k, Secret: def.Secret, Default: def.Default}

		switch {
		case val == "":
			if def.Required {
				r.OK = false
				r.Detail = "expected: present\n  actual:   missing (required)"
			} else if def.Default != "" {
				r.OK = true
			} else {
				r.OK = true
			}
		default:
			r.OK = true
			switch def.Type {
			case "integer":
				if _, err := strconv.Atoi(val); err != nil {
					r.OK = false
					r.Detail = "expected: type integer\n  actual:   " + val
				}
			case "boolean":
				if val != "true" && val != "false" {
					r.OK = false
					r.Detail = "expected: type boolean\n  actual:   " + val
				}
			case "float":
				if _, err := strconv.ParseFloat(val, 64); err != nil {
					r.OK = false
					r.Detail = "expected: type float\n  actual:   " + val
				}
			}
			if r.OK && len(def.Enum) > 0 && !contains(def.Enum, val) {
				r.OK = false
				r.Detail = fmt.Sprintf("expected: one of [%s]\n  actual:   %s", strings.Join(def.Enum, ", "), val)
			}
		}
		out = append(out, r)
	}
	return out
}

// Render formats validation results like the reference output.
func Render(envName string, results []Result) string {
	var b strings.Builder
	b.WriteString("Environment: " + envName + "\n\n")
	errs := 0
	for _, r := range results {
		if !r.OK {
			errs++
		}
	}
	for _, r := range results {
		if r.OK {
			b.WriteString("✓ " + r.Name + "\n")
			continue
		}
		b.WriteString("✗ " + r.Name + "\n\n")
		b.WriteString("  " + r.Detail + "\n")
	}
	b.WriteString("\n")
	if errs == 1 {
		b.WriteString("1 error")
	} else {
		b.WriteString(strconv.Itoa(errs) + " errors")
	}
	b.WriteString("\n")
	return b.String()
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
