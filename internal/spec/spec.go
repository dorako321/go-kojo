// Package spec defines the one-page order sheet a product is built from,
// and the acceptance cases the factory inspects it against.
package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Limits of the "S class" product: one file, few flags, few dependencies.
const (
	MaxLines = 200
	MaxFlags = 3
	MaxDeps  = 3
	MinCases = 2
)

// Kinds a product can be. The kind decides how strictly output can be checked.
var Kinds = []string{"convert", "inspect", "generate", "display", "fetch"}

type Spec struct {
	Name     string   `json:"name"`
	Summary  string   `json:"summary"`
	Kind     string   `json:"kind"`
	Usage    string   `json:"usage"`
	Flags    []Flag   `json:"flags"`
	Behavior []string `json:"behavior"`
	Deps     []string `json:"deps"`
	Network  bool     `json:"network"`
	Cases    []Case   `json:"cases"`
}

type Flag struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default string `json:"default"`
	Help    string `json:"help"`
}

type File struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Case is one acceptance case: run the program with Args and Stdin in a
// directory holding Files, then compare exit code and both output streams.
type Case struct {
	Name   string   `json:"name"`
	Args   []string `json:"args"`
	Stdin  string   `json:"stdin"`
	Files  []File   `json:"files"`
	Exit   int      `json:"exit"`
	Stdout Expect   `json:"stdout"`
	Stderr Expect   `json:"stderr"`
}

// Expect describes what one output stream must look like. Every field that
// is set must hold; an empty Expect accepts anything.
type Expect struct {
	Exact    *string  `json:"exact,omitempty"`
	Contains []string `json:"contains,omitempty"`
	Regex    string   `json:"regex,omitempty"`
	MinLines int      `json:"min_lines,omitempty"`
}

// Check returns one message per expectation that got does not meet.
func (e Expect) Check(got string) []string {
	var bad []string
	if e.Exact != nil && got != *e.Exact {
		bad = append(bad, fmt.Sprintf("want exactly %q, got %q", *e.Exact, clip(got)))
	}
	for _, sub := range e.Contains {
		if !strings.Contains(got, sub) {
			bad = append(bad, fmt.Sprintf("want it to contain %q, got %q", sub, clip(got)))
		}
	}
	if e.Regex != "" {
		re, err := regexp.Compile(e.Regex)
		if err != nil {
			bad = append(bad, fmt.Sprintf("bad regex %q: %v", e.Regex, err))
		} else if !re.MatchString(got) {
			bad = append(bad, fmt.Sprintf("want it to match %q, got %q", e.Regex, clip(got)))
		}
	}
	if n := strings.Count(got, "\n"); n < e.MinLines {
		bad = append(bad, fmt.Sprintf("want at least %d lines, got %d", e.MinLines, n))
	}
	return bad
}

func clip(s string) string {
	const max = 600
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(truncated)"
}

// Contract returns the cases every product must pass regardless of its spec.
func Contract() []Case {
	return []Case{
		{Name: "contract/help", Args: []string{"-h"}, Exit: 0, Stdout: Expect{Contains: []string{"Usage:"}}},
		{Name: "contract/version", Args: []string{"-version"}, Exit: 0, Stdout: Expect{Regex: `^\S+\n$`}},
		{Name: "contract/unknown-flag", Args: []string{"-kojo-no-such-flag"}, Exit: 2, Stderr: Expect{MinLines: 1}},
	}
}

var (
	nameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,19}$`)
	fileRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// Validate reports why a spec is outside what the factory can build.
func (s Spec) Validate() error {
	var bad []string
	add := func(format string, a ...any) { bad = append(bad, fmt.Sprintf(format, a...)) }

	if !nameRe.MatchString(s.Name) {
		add("name %q must be 2-20 chars of lowercase letters, digits and '-'", s.Name)
	}
	if strings.TrimSpace(s.Summary) == "" {
		add("summary is empty")
	}
	if !contains(Kinds, s.Kind) {
		add("kind %q must be one of %s", s.Kind, strings.Join(Kinds, ", "))
	}
	if strings.TrimSpace(s.Usage) == "" {
		add("usage is empty")
	}
	for _, v := range []string{s.Summary, s.Usage} {
		if strings.ContainsAny(v, "`\n") {
			add("summary and usage must be one line without backticks: %q", v)
		}
	}
	if len(s.Behavior) == 0 {
		add("behavior is empty")
	}
	if len(s.Flags) > MaxFlags {
		add("%d flags, the limit is %d", len(s.Flags), MaxFlags)
	}
	for _, f := range s.Flags {
		if f.Name == "h" || f.Name == "help" || f.Name == "version" {
			add("flag -%s is reserved by the factory", f.Name)
		}
	}
	if len(s.Deps) > MaxDeps {
		add("%d dependencies, the limit is %d", len(s.Deps), MaxDeps)
	}
	if len(s.Cases) < MinCases {
		add("%d cases, at least %d are required", len(s.Cases), MinCases)
	}
	seen := map[string]bool{}
	for _, c := range s.Cases {
		if c.Name == "" || seen[c.Name] {
			add("case name %q is empty or duplicated", c.Name)
		}
		seen[c.Name] = true
		for _, e := range []Expect{c.Stdout, c.Stderr} {
			if e.Regex != "" {
				if _, err := regexp.Compile(e.Regex); err != nil {
					add("case %q: bad regex %q", c.Name, e.Regex)
				}
			}
		}
		for _, f := range c.Files {
			if !fileRe.MatchString(f.Name) {
				add("case %q: file name %q must be a plain file name", c.Name, f.Name)
			}
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("spec is out of range:\n  - %s", strings.Join(bad, "\n  - "))
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Load reads and validates a spec file.
func Load(path string) (Spec, error) {
	var s Spec
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	return s, s.Validate()
}
