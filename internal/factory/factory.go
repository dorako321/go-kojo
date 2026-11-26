// Package factory runs one product down the line: stamp the frame, have the
// engine write main.go, inspect it, and send it back for repair a bounded
// number of times. It never touches git or GitHub.
package factory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dorako321/go-kojo/internal/engine"
	"github.com/dorako321/go-kojo/internal/inspect"
	"github.com/dorako321/go-kojo/internal/mold"
	"github.com/dorako321/go-kojo/internal/spec"
)

// Stations a product passes, in order.
const (
	StageScaffold = "scaffold"
	StageBuild    = "build"
	StageInspect  = "inspect"
	StageRepair   = "repair"
	StageAssemble = "assemble"
)

// Event is progress for whoever is watching the line.
type Event struct {
	Stage  string
	State  string // start | ok | fail | gate
	Text   string
	Gate   *inspect.Gate
	Source string // main.go as written, on a successful build or repair
}

type Options struct {
	Engine     engine.Engine
	Model      string // writes main.go and makes the first repairs
	Rescue     string // if set, makes the last repair: a stronger model for what the first could not fix
	Owner      string // GitHub owner, used for the module path
	Holder     string // copyright holder for LICENSE
	Request    string // the user's own words, kept in kojo.json
	MaxRepairs int
	NoDocs     bool   // do not show the engine `go doc` of the dependencies
	RunDir     string // prompts, replies and the built binary are kept here
	OnEvent    func(Event)
}

// Attempt is one call to the engine and what inspection made of the result.
type Attempt struct {
	Kind    string         `json:"kind"` // build | repair
	Model   string         `json:"model"`
	Seconds float64        `json:"seconds"`
	CostUSD float64        `json:"cost_usd"`
	Gates   []inspect.Gate `json:"gates,omitempty"`
}

// Report is the outcome for one product.
type Report struct {
	Name     string    `json:"name"`
	Status   string    `json:"status"` // ok | failed | cannot | dispute | error
	Reason   string    `json:"reason,omitempty"`
	Attempts []Attempt `json:"attempts"`
	Lines    int       `json:"lines"`
	CostUSD  float64   `json:"cost_usd"`
	Seconds  float64   `json:"seconds"`
	Dir      string    `json:"dir"`
	Binary   string    `json:"binary,omitempty"`
	Created  bool      `json:"-"` // Build made Dir itself, so it is the factory's to remove
}

// Repairs is how many times the product was sent back.
func (r Report) Repairs() int {
	if len(r.Attempts) == 0 {
		return 0
	}
	return len(r.Attempts) - 1
}

// FailedGates names the gates the last attempt did not pass.
func (r Report) FailedGates() []string {
	if len(r.Attempts) == 0 {
		return nil
	}
	var names []string
	for _, g := range r.Attempts[len(r.Attempts)-1].Gates {
		if !g.OK {
			names = append(names, g.Name)
		}
	}
	return names
}

type reply struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
	MainGo string `json:"main_go"`
}

const replySchema = `{"type":"object","properties":{` +
	`"status":{"type":"string","enum":["ok","cannot","dispute"]},` +
	`"reason":{"type":"string"},` +
	`"main_go":{"type":"string"}},` +
	`"required":["status","reason","main_go"]}`

// Build makes the product described by sp in dir, which must not exist yet.
func Build(ctx context.Context, sp spec.Spec, dir string, opt Options) (Report, error) {
	start := time.Now()
	rep := Report{Name: sp.Name, Status: "error", Dir: dir}
	finish := func(err error) (Report, error) {
		rep.Seconds = time.Since(start).Seconds()
		if err != nil {
			rep.Reason = err.Error()
		}
		return rep, err
	}
	emit := func(stage, state, text string) {
		if opt.OnEvent != nil {
			opt.OnEvent(Event{Stage: stage, State: state, Text: text})
		}
	}

	if err := sp.Validate(); err != nil {
		return finish(err)
	}
	if opt.Engine == nil {
		return finish(errors.New("no engine"))
	}
	if _, err := os.Stat(dir); err == nil {
		return finish(fmt.Errorf("%s already exists", dir))
	}
	if opt.RunDir == "" {
		tmp, err := os.MkdirTemp("", "kojo-run-")
		if err != nil {
			return finish(err)
		}
		opt.RunDir = tmp
	}
	if err := os.MkdirAll(filepath.Join(opt.RunDir, "bin"), 0o755); err != nil {
		return finish(err)
	}

	// Scaffold: everything that needs no model.
	emit(StageScaffold, "start", "")
	data := mold.Data{
		Spec:      sp,
		Module:    "github.com/" + opt.Owner + "/" + sp.Name,
		GoVersion: goLanguageVersion(ctx),
		Holder:    opt.Holder,
		Year:      time.Now().Year(),
		Request:   opt.Request,
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return finish(err)
	}
	rep.Created = true
	if err := mold.Scaffold(dir, data); err != nil {
		return finish(err)
	}
	frame, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		return finish(err)
	}
	docs, err := fetchDeps(ctx, dir, sp.Deps, !opt.NoDocs)
	if err != nil {
		emit(StageScaffold, "fail", err.Error())
		return finish(err)
	}
	emit(StageScaffold, "ok", fmt.Sprintf("frame ready, %d dependencies", len(sp.Deps)))

	prompt := buildPrompt(sp, string(frame), docs)
	source := ""
	var last inspect.Result

	for n := 0; n <= opt.MaxRepairs; n++ {
		kind, stage := "build", StageBuild
		if n > 0 {
			kind, stage = "repair", StageRepair
			prompt = repairPrompt(sp, string(frame), docs, source, last.Failed())
		}
		model := opt.Model
		if n > 0 && n == opt.MaxRepairs && opt.Rescue != "" {
			model = opt.Rescue
		}
		emit(stage, "start", model)

		logFile(opt.RunDir, fmt.Sprintf("%02d-%s.prompt.txt", n+1, kind), []byte(prompt))
		resp, err := opt.Engine.Call(ctx, engine.Request{
			System: houseRules, Prompt: prompt, Schema: replySchema, Model: model,
		})
		logFile(opt.RunDir, fmt.Sprintf("%02d-%s.reply.json", n+1, kind), resp.Raw)
		att := Attempt{Kind: kind, Model: model, Seconds: resp.Duration.Seconds(), CostUSD: resp.CostUSD}
		rep.CostUSD += resp.CostUSD
		if err != nil {
			rep.Attempts = append(rep.Attempts, att)
			emit(stage, "fail", err.Error())
			return finish(err)
		}
		var r reply
		if err := json.Unmarshal(resp.Output, &r); err != nil {
			rep.Attempts = append(rep.Attempts, att)
			return finish(fmt.Errorf("engine reply: %w", err))
		}
		if r.Status != "ok" {
			rep.Attempts = append(rep.Attempts, att)
			rep.Status, rep.Reason = r.Status, r.Reason
			emit(stage, "fail", r.Reason)
			return finish(nil)
		}

		// gofmt is applied mechanically; style is never a reason to repair.
		source = r.MainGo
		if formatted, err := format.Source([]byte(source)); err == nil {
			source = string(formatted)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644); err != nil {
			return finish(err)
		}
		if opt.OnEvent != nil {
			opt.OnEvent(Event{Stage: stage, State: "ok", Source: source,
				Text: fmt.Sprintf("main.go, %d lines, %.0fs", strings.Count(source, "\n"), resp.Duration.Seconds())})
		}

		emit(StageInspect, "start", "")
		last = inspect.Run(ctx, dir, sp, filepath.Join(opt.RunDir, "bin"), func(g inspect.Gate) {
			if opt.OnEvent != nil {
				opt.OnEvent(Event{Stage: StageInspect, State: "gate", Text: g.Name, Gate: &g})
			}
		})
		att.Gates = last.Gates
		rep.Attempts = append(rep.Attempts, att)
		rep.Lines = last.Lines
		gateLog, _ := json.MarshalIndent(last.Gates, "", "  ")
		logFile(opt.RunDir, fmt.Sprintf("%02d-%s.gates.json", n+1, kind), gateLog)

		if last.OK() {
			emit(StageInspect, "ok", fmt.Sprintf("%d gates passed", len(last.Gates)))
			rep.Binary = last.Binary
			emit(StageAssemble, "start", "")
			if err := assemble(ctx, dir, data, last.Binary, opt.Model); err != nil {
				emit(StageAssemble, "fail", err.Error())
				return finish(err)
			}
			emit(StageAssemble, "ok", "README written from real output")
			rep.Status = "ok"
			return finish(nil)
		}
		emit(StageInspect, "fail", strings.Join(gateNames(last.Failed()), ", "))
	}

	rep.Status = "failed"
	rep.Reason = "inspection still failing after " + fmt.Sprint(opt.MaxRepairs) + " repairs: " + strings.Join(gateNames(last.Failed()), ", ")
	return finish(nil)
}

func gateNames(gs []inspect.Gate) []string {
	var names []string
	for _, g := range gs {
		names = append(names, g.Name)
	}
	return names
}

// assemble writes the files that describe the finished program: the README
// from the binary's own output, and the manifest with where it came from.
func assemble(ctx context.Context, dir string, d mold.Data, bin, model string) error {
	help, err := inspect.Execute(ctx, bin, spec.Case{Args: []string{"-h"}})
	if err != nil {
		return err
	}
	var examples []mold.Example
	for _, c := range d.Spec.Cases {
		if len(examples) == 3 {
			break
		}
		if c.Exit != 0 || len(c.Files) > 0 {
			continue
		}
		out, err := inspect.Execute(ctx, bin, c)
		if err != nil || out.Stdout == "" || strings.ContainsRune(out.Stdout, '\x1b') || strings.Count(out.Stdout, "\n") > 20 {
			continue
		}
		examples = append(examples, mold.Example{Command: commandLine(d.Spec.Name, c), Output: out.Stdout})
	}
	if err := mold.WriteReadme(dir, d, help.Stdout, examples); err != nil {
		return err
	}
	return mold.WriteManifest(dir, mold.Manifest{Factory: "go-kojo", Request: d.Request, Model: model, Spec: d.Spec})
}

// commandLine shows a case the way a person would type it.
func commandLine(name string, c spec.Case) string {
	quote := func(s string) string {
		if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`|&;<>()*?[]{}~#!") {
			return s
		}
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	parts := []string{name}
	for _, a := range c.Args {
		parts = append(parts, quote(a))
	}
	cmd := strings.Join(parts, " ")
	if c.Stdin == "" {
		return cmd
	}
	if body, ok := strings.CutSuffix(c.Stdin, "\n"); ok && !strings.ContainsAny(body, "\n\\") {
		return "echo " + quote(body) + " | " + cmd
	}
	escaped := strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "'", `'\''`, "%", "%%").Replace(c.Stdin)
	return "printf '" + escaped + "' | " + cmd
}

// fetchDeps downloads the declared dependencies and, unless told not to,
// returns their `go doc` so the engine writes against the real API.
func fetchDeps(ctx context.Context, dir string, deps []string, withDocs bool) (string, error) {
	var docs strings.Builder
	for _, dep := range deps {
		if out, err := goCmd(ctx, dir, "get", dep+"@latest"); err != nil {
			return "", fmt.Errorf("dependency %s could not be fetched: %s", dep, lastLine(out))
		}
		if !withDocs {
			continue
		}
		out, err := goCmd(ctx, dir, "doc", "-all", dep)
		if err != nil {
			continue
		}
		const max = 12000
		if len(out) > max {
			out = out[:max] + "\n…(truncated)"
		}
		fmt.Fprintf(&docs, "\n### go doc -all %s\n\n%s\n", dep, out)
	}
	return docs.String(), nil
}

func goLanguageVersion(ctx context.Context) string {
	out, err := goCmd(ctx, "", "env", "GOVERSION")
	v := strings.TrimPrefix(strings.TrimSpace(out), "go")
	parts := strings.Split(v, ".")
	if err != nil || len(parts) < 2 {
		return "1.24"
	}
	return parts[0] + "." + parts[1]
}

func goCmd(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func logFile(runDir, name string, body []byte) {
	if runDir != "" && len(body) > 0 {
		_ = os.WriteFile(filepath.Join(runDir, name), body, 0o644)
	}
}
