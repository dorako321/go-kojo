// Package mold stamps out everything in a product repository that does not
// need a language model: the frame of main.go, the tests generated from the
// acceptance cases, and the fixed files around them.
package mold

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/dorako321/go-kojo/internal/spec"
)

//go:embed templates/*
var files embed.FS

var tmpl = template.Must(template.ParseFS(files, "templates/*"))

// Data is what the fixed files are filled in with.
type Data struct {
	Spec      spec.Spec
	Module    string // e.g. github.com/owner/name
	GoVersion string // language version for go.mod, e.g. 1.27
	Holder    string // copyright holder
	Year      int
	Request   string // the words the product was ordered with
}

// Example is one real run of the built program, shown in the README.
type Example struct {
	Command string
	Output  string
}

func render(name string, data any) ([]byte, error) {
	var b bytes.Buffer
	if err := tmpl.ExecuteTemplate(&b, name, data); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Frame returns main.go with the contract in place and no behaviour yet.
func Frame(d Data) ([]byte, error) {
	src, err := render("main.go.tmpl", d)
	if err != nil {
		return nil, err
	}
	return format.Source(src)
}

// TestFile returns main_test.go: a fixed runner plus one table row per case.
func TestFile(sp spec.Spec) ([]byte, error) {
	runner, err := render("main_test.go.tmpl", nil)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.Write(runner)
	b.WriteString("\nvar kojoCases = []kojoCase{\n")
	for _, c := range sp.Cases {
		fmt.Fprintf(&b, "{\nname: %q,\n", c.Name)
		if len(c.Args) > 0 {
			fmt.Fprintf(&b, "args: %#v,\n", c.Args)
		}
		if c.Stdin != "" {
			fmt.Fprintf(&b, "stdin: %q,\n", c.Stdin)
		}
		if len(c.Files) > 0 {
			b.WriteString("files: [][2]string{")
			for _, f := range c.Files {
				fmt.Fprintf(&b, "{%q, %q},", f.Name, f.Content)
			}
			b.WriteString("},\n")
		}
		fmt.Fprintf(&b, "exit: %d,\n", c.Exit)
		if lit := expectLit(c.Stdout); lit != "" {
			fmt.Fprintf(&b, "stdout: %s,\n", lit)
		}
		if lit := expectLit(c.Stderr); lit != "" {
			fmt.Fprintf(&b, "stderr: %s,\n", lit)
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n")
	return format.Source([]byte(b.String()))
}

func expectLit(e spec.Expect) string {
	var parts []string
	if e.Exact != nil {
		parts = append(parts, fmt.Sprintf("exact: kojoStr(%q)", *e.Exact))
	}
	if len(e.Contains) > 0 {
		parts = append(parts, fmt.Sprintf("contains: %#v", e.Contains))
	}
	if e.Regex != "" {
		parts = append(parts, fmt.Sprintf("regex: %q", e.Regex))
	}
	if e.MinLines > 0 {
		parts = append(parts, fmt.Sprintf("minLines: %d", e.MinLines))
	}
	if len(parts) == 0 {
		return ""
	}
	return "kojoExpect{" + strings.Join(parts, ", ") + "}"
}

// Manifest is kojo.json: where a product came from and what it was held to.
type Manifest struct {
	Factory string    `json:"factory"`
	Request string    `json:"request,omitempty"`
	Model   string    `json:"model,omitempty"`
	Spec    spec.Spec `json:"spec"`
}

// Scaffold writes the product directory with a frame-only main.go.
func Scaffold(dir string, d Data) error {
	frame, err := Frame(d)
	if err != nil {
		return fmt.Errorf("frame: %w", err)
	}
	test, err := TestFile(d.Spec)
	if err != nil {
		return fmt.Errorf("test file: %w", err)
	}
	license, err := render("LICENSE.tmpl", d)
	if err != nil {
		return err
	}
	ci, err := render("ci.yml.tmpl", d)
	if err != nil {
		return err
	}
	out := map[string][]byte{
		"go.mod":                   []byte(fmt.Sprintf("module %s\n\ngo %s\n", d.Module, d.GoVersion)),
		"main.go":                  frame,
		"main_test.go":             test,
		"LICENSE":                  license,
		".gitignore":               []byte("/" + d.Spec.Name + "\n*.exe\n.DS_Store\n"),
		".github/workflows/ci.yml": ci,
	}
	for name, body := range out {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return err
		}
	}
	return WriteManifest(dir, Manifest{Factory: "go-kojo", Request: d.Request, Spec: d.Spec})
}

func WriteManifest(dir string, m Manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "kojo.json"), append(b, '\n'), 0o644)
}

// WriteReadme fills the README with the program's own help text and with
// output captured from real runs, so nothing in it is written from memory.
func WriteReadme(dir string, d Data, help string, examples []Example) error {
	if !strings.HasSuffix(help, "\n") {
		help += "\n"
	}
	for i := range examples {
		if !strings.HasSuffix(examples[i].Output, "\n") {
			examples[i].Output += "\n"
		}
	}
	b, err := render("README.md.tmpl", struct {
		Data
		Help     string
		Examples []Example
	}{d, help, examples})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "README.md"), b, 0o644)
}
