// Package eval builds the same specs many times and counts what happens,
// so changes to the factory are judged by numbers rather than by one run.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dorako321/go-kojo/internal/engine"
	"github.com/dorako321/go-kojo/internal/factory"
	"github.com/dorako321/go-kojo/internal/spec"
)

type Config struct {
	Specs      []string          // spec files
	Models     []string          // full model IDs
	Runs       int               // builds per spec and model
	Parallel   int               // builds running at once
	MaxRepairs int               // repairs allowed per build
	NoDocs     bool              // withhold dependency docs from the engine
	OutDir     string            // products, run logs and results.jsonl
	Verify     map[string]string // spec name -> directory of a Go program that checks the binary
	Engine     engine.Engine
	Log        io.Writer
}

// Row is one build.
type Row struct {
	Spec     string   `json:"spec"`
	Model    string   `json:"model"`
	Docs     bool     `json:"docs"`
	Run      int      `json:"run"`
	Status   string   `json:"status"`
	FirstTry bool     `json:"first_try"`
	Repairs  int      `json:"repairs"`
	Seconds  float64  `json:"seconds"`
	CostUSD  float64  `json:"cost_usd"`
	Lines    int      `json:"lines"`
	Failed   []string `json:"failed_gates,omitempty"`
	Verify   string   `json:"verify,omitempty"` // ok | fail, when a verifier is set
	Detail   string   `json:"detail,omitempty"`
	Dir      string   `json:"dir"`
}

// Passed is true when the product passed inspection and, if one was set, the
// independent verifier too.
func (r Row) Passed() bool { return r.Status == "ok" && r.Verify != "fail" }

func Run(ctx context.Context, cfg Config) ([]Row, error) {
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return nil, err
	}
	out, err := filepath.Abs(cfg.OutDir)
	if err != nil {
		return nil, err
	}

	// Verifiers are compiled once, before any build can race for them.
	verifiers := map[string]string{}
	for name, dir := range cfg.Verify {
		bin := filepath.Join(out, "verify-"+name)
		cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, ".")
		cmd.Dir = dir
		if b, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("verifier %s: %v\n%s", dir, err, b)
		}
		verifiers[name] = bin
	}

	type job struct {
		sp    spec.Spec
		model string
		run   int
	}
	var jobs []job
	for _, path := range cfg.Specs {
		sp, err := spec.Load(path)
		if err != nil {
			return nil, err
		}
		for _, m := range cfg.Models {
			for i := 1; i <= cfg.Runs; i++ {
				jobs = append(jobs, job{sp, m, i})
			}
		}
	}

	results, err := os.OpenFile(filepath.Join(out, "results.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	defer results.Close()

	var (
		mu   sync.Mutex
		rows []Row
		wg   sync.WaitGroup
		sem  = make(chan struct{}, max(cfg.Parallel, 1))
	)
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			id := fmt.Sprintf("%s-%s-%02d", j.sp.Name, shortModel(j.model), j.run)
			rep, _ := factory.Build(ctx, j.sp, filepath.Join(out, id, j.sp.Name), factory.Options{
				Engine: cfg.Engine, Model: j.model, Owner: "example", Holder: "go-kojo eval",
				MaxRepairs: cfg.MaxRepairs, NoDocs: cfg.NoDocs, RunDir: filepath.Join(out, id, "run"),
			})
			row := Row{
				Spec: j.sp.Name, Model: j.model, Docs: !cfg.NoDocs, Run: j.run,
				Status: rep.Status, Repairs: rep.Repairs(), FirstTry: rep.Status == "ok" && rep.Repairs() == 0,
				Seconds: rep.Seconds, CostUSD: rep.CostUSD, Lines: rep.Lines,
				Failed: rep.FailedGates(), Detail: rep.Reason, Dir: rep.Dir,
			}
			if v := verifiers[j.sp.Name]; v != "" && rep.Status == "ok" {
				vctx, cancel := context.WithTimeout(ctx, time.Minute)
				b, err := exec.CommandContext(vctx, v, rep.Binary).CombinedOutput()
				cancel()
				row.Verify = "ok"
				if err != nil {
					row.Verify, row.Detail = "fail", strings.TrimSpace(string(b))
				}
			}

			mu.Lock()
			defer mu.Unlock()
			rows = append(rows, row)
			line, _ := json.Marshal(row)
			results.Write(append(line, '\n'))
			fmt.Fprintf(cfg.Log, "%-28s %-7s repairs=%d verify=%-4s %5.0fs $%.3f %3d lines %s\n",
				id, row.Status, row.Repairs, dash(row.Verify), row.Seconds, row.CostUSD, row.Lines, strings.Join(row.Failed, ","))
		}()
	}
	wg.Wait()
	Summarize(cfg.Log, rows)
	return rows, nil
}

// Summarize prints one line per spec and model.
func Summarize(w io.Writer, rows []Row) {
	type key struct{ spec, model string }
	groups := map[key][]Row{}
	for _, r := range rows {
		k := key{r.Spec, r.Model}
		groups[k] = append(groups[k], r)
	}
	keys := make([]key, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].spec+keys[i].model < keys[j].spec+keys[j].model
	})
	fmt.Fprintf(w, "\n%-10s %-20s %6s %10s %8s %8s %8s\n", "spec", "model", "passed", "first try", "avg sec", "avg $", "avg loc")
	for _, k := range keys {
		g := groups[k]
		var passed, first int
		var sec, cost, lines float64
		for _, r := range g {
			if r.Passed() {
				passed++
			}
			if r.FirstTry && r.Passed() {
				first++
			}
			sec += r.Seconds
			cost += r.CostUSD
			lines += float64(r.Lines)
		}
		n := float64(len(g))
		fmt.Fprintf(w, "%-10s %-20s %3d/%-2d %7d/%-2d %8.0f %8.3f %8.0f\n",
			k.spec, k.model, passed, len(g), first, len(g), sec/n, cost/n, lines/n)
	}
}

func shortModel(m string) string {
	m = strings.TrimPrefix(m, "claude-")
	if i := strings.Index(m, "-"); i > 0 {
		return m[:i]
	}
	return m
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
