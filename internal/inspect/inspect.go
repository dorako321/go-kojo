// Package inspect decides whether a product may leave the factory. Every
// gate is plain Go code and shell-free commands; no language model is asked
// whether its own work is good.
package inspect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dorako321/go-kojo/internal/spec"
)

// Gate is the outcome of one inspection step. Hint tells whoever repairs the
// product what to change, not just what went wrong.
type Gate struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	Hint   string `json:"hint,omitempty"`
}

type Result struct {
	Gates  []Gate `json:"gates"`
	Binary string `json:"binary,omitempty"`
	Lines  int    `json:"lines"`
}

func (r Result) OK() bool { return len(r.Gates) > 0 && len(r.Failed()) == 0 }

func (r Result) Failed() []Gate {
	var bad []Gate
	for _, g := range r.Gates {
		if !g.OK {
			bad = append(bad, g)
		}
	}
	return bad
}

// Run inspects the product in dir and leaves the built binary in binDir.
// report, if not nil, is called as each gate finishes.
func Run(ctx context.Context, dir string, sp spec.Spec, binDir string, report func(Gate)) Result {
	var res Result
	add := func(g Gate) bool {
		res.Gates = append(res.Gates, g)
		if report != nil {
			report(g)
		}
		return g.OK
	}

	src, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		add(Gate{Name: "parse", Detail: err.Error()})
		return res
	}
	res.Lines = bytes.Count(src, []byte("\n"))

	// A program that does not parse or pulls in undeclared packages goes
	// no further: nothing is downloaded or executed for it.
	if !add(gateImports(src, sp)) {
		return res
	}
	add(gateLines(res.Lines))
	add(gateFmt(ctx, dir))
	if !add(gateTidy(ctx, dir, sp)) {
		return res
	}
	bin := filepath.Join(binDir, sp.Name)
	if !add(gateGo(ctx, dir, "build", "Fix the compile errors.", "build", "-o", bin, ".")) {
		return res
	}
	res.Binary = bin
	add(gateGo(ctx, dir, "vet", "Fix what go vet reports. main_test.go is fixed and calls run(args, stdin, stdout, stderr) int; keep that signature and do not declare identifiers starting with \"kojo\".", "vet", "./..."))
	if !add(gateCases(ctx, bin, sp)) {
		return res
	}
	// The repository's own test runs the same cases through run() directly.
	// It can only disagree with the binary when run() bypasses its arguments.
	add(gateGo(ctx, dir, "test", "These cases pass against the built binary but fail under go test, which calls run() directly. Inside run, read only from the stdin parameter, write only to the stdout and stderr parameters, and never call os.Exit.", "test", "./..."))
	return res
}

func gateLines(n int) Gate {
	g := Gate{Name: "lines", OK: n <= spec.MaxLines}
	if !g.OK {
		g.Detail = fmt.Sprintf("main.go has %d lines, the limit is %d", n, spec.MaxLines)
		g.Hint = "Make the program shorter: remove anything the numbered behaviors do not ask for."
	}
	return g
}

var forbidden = map[string]string{
	"os/exec": "running other programs is not allowed",
	"syscall": "direct system calls are not allowed",
	"unsafe":  "unsafe is not allowed",
	"plugin":  "plugins are not allowed",
}

func gateImports(src []byte, sp spec.Spec) Gate {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", src, parser.ImportsOnly)
	if err != nil {
		return Gate{Name: "parse", Detail: err.Error(), Hint: "main.go must be one complete, valid Go source file."}
	}
	var bad []string
	for _, im := range f.Imports {
		path, _ := strconv.Unquote(im.Path.Value)
		switch {
		case forbidden[path] != "":
			bad = append(bad, fmt.Sprintf("%s: %s", path, forbidden[path]))
		case !sp.Network && (path == "net" || strings.HasPrefix(path, "net/")):
			bad = append(bad, fmt.Sprintf("%s: this product is specified as not using the network", path))
		case strings.Contains(strings.SplitN(path, "/", 2)[0], ".") && !declared(sp.Deps, path):
			bad = append(bad, fmt.Sprintf("%s: not among the declared dependencies", path))
		}
	}
	g := Gate{Name: "imports", OK: len(bad) == 0}
	if !g.OK {
		g.Detail = strings.Join(bad, "\n")
		g.Hint = "Use only the standard library and the declared dependencies: " + depList(sp.Deps) + "."
	}
	return g
}

func declared(deps []string, path string) bool {
	for _, d := range deps {
		if path == d || strings.HasPrefix(path, d+"/") {
			return true
		}
	}
	return false
}

func depList(deps []string) string {
	if len(deps) == 0 {
		return "(none declared)"
	}
	return strings.Join(deps, ", ")
}

func gateFmt(ctx context.Context, dir string) Gate {
	out, err := run(ctx, dir, time.Minute, "gofmt", "-l", ".")
	g := Gate{Name: "gofmt", OK: err == nil && strings.TrimSpace(out) == ""}
	if !g.OK {
		g.Detail = strings.TrimSpace(out)
		g.Hint = "Format the source the way gofmt does."
	}
	return g
}

func gateTidy(ctx context.Context, dir string, sp spec.Spec) Gate {
	if out, err := goCmd(ctx, dir, "mod", "tidy"); err != nil {
		return Gate{Name: "deps", Detail: clip(out), Hint: "Import only packages that exist in the declared dependencies: " + depList(sp.Deps) + "."}
	}
	out, err := goCmd(ctx, dir, "mod", "edit", "-json")
	if err != nil {
		return Gate{Name: "deps", Detail: clip(out)}
	}
	var mod struct {
		Require []struct {
			Path     string
			Indirect bool
		}
	}
	if err := json.Unmarshal([]byte(out), &mod); err != nil {
		return Gate{Name: "deps", Detail: err.Error()}
	}
	var bad []string
	for _, r := range mod.Require {
		ok := false
		for _, d := range sp.Deps {
			if d == r.Path || strings.HasPrefix(d, r.Path+"/") || strings.HasPrefix(r.Path, d+"/") {
				ok = true
			}
		}
		if !r.Indirect && !ok {
			bad = append(bad, r.Path)
		}
	}
	g := Gate{Name: "deps", OK: len(bad) == 0}
	if !g.OK {
		g.Detail = "go.mod requires modules the spec does not declare: " + strings.Join(bad, ", ")
		g.Hint = "Use only the declared dependencies: " + depList(sp.Deps) + "."
	}
	return g
}

func gateGo(ctx context.Context, dir, name, hint string, args ...string) Gate {
	out, err := goCmd(ctx, dir, args...)
	g := Gate{Name: name, OK: err == nil}
	if !g.OK {
		g.Detail = clip(out)
		g.Hint = hint
	}
	return g
}

func gateCases(ctx context.Context, bin string, sp spec.Spec) Gate {
	var bad []string
	all := append(spec.Contract(), sp.Cases...)
	for _, c := range all {
		if problems := CheckCase(ctx, bin, c); len(problems) > 0 {
			bad = append(bad, fmt.Sprintf("case %q (args %q, stdin %q):\n    %s",
				c.Name, c.Args, c.Stdin, strings.Join(problems, "\n    ")))
		}
	}
	g := Gate{Name: "cases", OK: len(bad) == 0}
	if !g.OK {
		g.Detail = fmt.Sprintf("%d of %d cases failed\n  %s", len(bad), len(all), strings.Join(bad, "\n  "))
		g.Hint = "Make the program follow the numbered behaviors so these cases pass. Do not recognise the case inputs specially. Cases named contract/* are the frame's -h, -version and unknown-flag handling; keep that code as given."
	}
	return g
}

// Output is what one run of the built program produced.
type Output struct {
	Exit   int
	Stdout string
	Stderr string
}

// Execute runs the built program on one case in a scratch directory with a
// minimal environment, so it sees none of the caller's files or variables.
func Execute(ctx context.Context, bin string, c spec.Case) (Output, error) {
	dir, err := os.MkdirTemp("", "kojo-case-")
	if err != nil {
		return Output{}, err
	}
	defer os.RemoveAll(dir)
	for _, f := range c.Files {
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(f.Name)), []byte(f.Content), 0o644); err != nil {
			return Output{}, err
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, c.Args...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "TMPDIR=" + dir, "LANG=en_US.UTF-8"}
	cmd.Stdin = strings.NewReader(c.Stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second

	err = cmd.Run()
	out := Output{Stdout: stdout.String(), Stderr: stderr.String()}
	var exit *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return out, errors.New("timed out after 10s; the program must never wait for input it was not given")
	case errors.As(err, &exit):
		out.Exit = exit.ExitCode()
	case err != nil:
		return out, err
	}
	return out, nil
}

// CheckCase returns what a run got wrong, or nil when the case passes.
func CheckCase(ctx context.Context, bin string, c spec.Case) []string {
	out, err := Execute(ctx, bin, c)
	if err != nil {
		return []string{err.Error()}
	}
	var bad []string
	if out.Exit != c.Exit {
		bad = append(bad, fmt.Sprintf("exit code: want %d, got %d (stderr %q)", c.Exit, out.Exit, clip(out.Stderr)))
	}
	for _, p := range c.Stdout.Check(out.Stdout) {
		bad = append(bad, "stdout: "+p)
	}
	for _, p := range c.Stderr.Check(out.Stderr) {
		bad = append(bad, "stderr: "+p)
	}
	return bad
}

func goCmd(ctx context.Context, dir string, args ...string) (string, error) {
	return run(ctx, dir, 5*time.Minute, "go", args...)
}

func run(ctx context.Context, dir string, limit time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0", "GOFLAGS=")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	const max = 3000
	if len(s) > max {
		return s[:max] + "\n…(truncated)"
	}
	return s
}
