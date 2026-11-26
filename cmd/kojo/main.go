// kojo is a small factory for single-purpose command-line programs.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/dorako321/go-kojo/internal/demo"
	"github.com/dorako321/go-kojo/internal/engine"
	"github.com/dorako321/go-kojo/internal/eval"
	"github.com/dorako321/go-kojo/internal/factory"
	"github.com/dorako321/go-kojo/internal/mix"
	"github.com/dorako321/go-kojo/internal/order"
	"github.com/dorako321/go-kojo/internal/spec"
	"github.com/dorako321/go-kojo/internal/tui"
)

// Who does what by default. Two models share the work: one that is quick and
// writes a lot, and a stronger one that is asked only for short, hard
// judgements. A smaller model was tried for reception and was cheaper but
// not quicker, so it is not used.
const (
	workModel   = "claude-sonnet-5-5"
	strongModel = "claude-opus-5-5"
)

const defaultModel = workModel

const usage = `kojo - a factory for small command-line programs

Usage:
  kojo                  open the factory floor and order a program by talking
  kojo mix [seed]       let the factory invent products: it forces ideas together, picks 3 and builds them
  kojo demo             watch a scripted order go down the line; no model is called
  kojo demo mix         watch a recorded run of the planning room; no model is called
  kojo build -spec FILE [-dir DIR] [-model ID]            build one product from a spec file
  kojo eval  -spec FILE[,FILE] -out DIR [-n 5] [-model ID[,ID]]   build specs repeatedly and count results
  kojo eval  -orders FILE -out DIR [-arms solo,team]      take written orders all the way to products

The staff can be changed with -reception, -design, -review, -build, -rescue,
-plan and -judge (model IDs; "-review none" and "-rescue none" leave those out).
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:]))
}

func run(ctx context.Context, args []string) int {
	// With no command, or only flags, kojo opens the floor.
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	var err error
	switch {
	case cmd == "-h" || cmd == "--help" || cmd == "help":
		fmt.Print(usage)
		return 0
	case cmd == "" || strings.HasPrefix(cmd, "-"):
		err = cmdFloor(args)
	case cmd == "mix":
		err = cmdMix(ctx, args[1:])
	case cmd == "demo":
		err = cmdDemo(len(args) > 1 && args[1] == "mix")
	case cmd == "build":
		err = cmdBuild(ctx, args[1:])
	case cmd == "eval":
		err = cmdEval(ctx, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "kojo: unknown command %q\n%s", cmd, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "kojo:", err)
		return 1
	}
	return 0
}

func cmdBuild(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	specPath := fs.String("spec", "", "spec file (JSON)")
	dir := fs.String("dir", "", "directory to create (default ./<name>)")
	model := fs.String("model", defaultModel, "model ID")
	repairs := fs.Int("repairs", 2, "repairs allowed after a failed inspection")
	noDocs := fs.Bool("no-docs", false, "do not show dependency docs to the engine")
	rescue := fs.String("rescue", "none", "model for the last repair, or none")
	owner := fs.String("owner", "", "GitHub owner for the module path (default: gh login)")
	fs.Parse(args)

	sp, err := spec.Load(*specPath)
	if err != nil {
		return err
	}
	eng, err := engine.NewClaude()
	if err != nil {
		return err
	}
	if *dir == "" {
		*dir = sp.Name
	}
	if *owner == "" {
		*owner = firstLine("gh", "api", "user", "-q", ".login")
	}
	if *owner == "" {
		return fmt.Errorf("could not tell the GitHub owner; pass -owner")
	}
	runDir, err := os.MkdirTemp("", "kojo-run-")
	if err != nil {
		return err
	}

	rep, err := factory.Build(ctx, sp, *dir, factory.Options{
		Engine: eng, Model: *model, Rescue: orNone(*rescue), Owner: *owner, Holder: firstLine("git", "config", "user.name"),
		MaxRepairs: *repairs, NoDocs: *noDocs, RunDir: runDir,
		OnEvent: func(e factory.Event) {
			switch {
			case e.Gate != nil && e.Gate.OK:
				fmt.Printf("    ok    %s\n", e.Gate.Name)
			case e.Gate != nil:
				fmt.Printf("    FAIL  %s\n%s\n", e.Gate.Name, indent(e.Gate.Detail))
			default:
				fmt.Printf("%-9s %-5s %s\n", e.Stage, e.State, e.Text)
			}
		},
	})
	fmt.Printf("\n%s: %s  (%d repairs, %.0fs, $%.3f, %d lines)\n  product %s\n  run log %s\n",
		rep.Name, rep.Status, rep.Repairs(), rep.Seconds, rep.CostUSD, rep.Lines, rep.Dir, runDir)
	if err != nil {
		return err
	}
	if rep.Status != "ok" {
		return fmt.Errorf("%s", rep.Reason)
	}
	return nil
}

func cmdEval(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("eval", flag.ExitOnError)
	specs := fs.String("spec", "", "spec files, comma separated")
	models := fs.String("model", defaultModel, "model IDs, comma separated")
	n := fs.Int("n", 5, "builds per spec and model")
	parallel := fs.Int("parallel", 4, "builds running at once")
	repairs := fs.Int("repairs", 2, "repairs allowed after a failed inspection")
	noDocs := fs.Bool("no-docs", false, "do not show dependency docs to the engine")
	out := fs.String("out", "", "directory for products and results.jsonl")
	verify := fs.String("verify", "", "extra checks, as name=dir[,name=dir]; dir holds a Go program given the binary path")
	orders := fs.String("orders", "", "file of orders, one per line, to take from words to products")
	armNames := fs.String("arms", "solo,team", "with -orders: ways of staffing the factory to compare")
	fs.Parse(args)

	if (*specs == "") == (*orders == "") || *out == "" {
		return fmt.Errorf("eval needs -out and one of -spec or -orders")
	}
	eng, err := engine.NewClaude()
	if err != nil {
		return err
	}
	if *orders != "" {
		text, err := os.ReadFile(*orders)
		if err != nil {
			return err
		}
		cfg := eval.OrderConfig{Parallel: *parallel, OutDir: *out, Engine: eng, Log: os.Stdout}
		for _, line := range strings.Split(string(text), "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
				cfg.Orders = append(cfg.Orders, line)
			}
		}
		for _, name := range strings.Split(*armNames, ",") {
			arm, ok := arms[name]
			if !ok {
				return fmt.Errorf("unknown arm %q", name)
			}
			cfg.Arms = append(cfg.Arms, arm)
		}
		_, err = eval.RunOrders(ctx, cfg)
		return err
	}
	verifiers := map[string]string{}
	for _, kv := range strings.Split(*verify, ",") {
		if name, dir, ok := strings.Cut(kv, "="); ok {
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			verifiers[name] = abs
		}
	}
	_, err = eval.Run(ctx, eval.Config{
		Specs: strings.Split(*specs, ","), Models: strings.Split(*models, ","),
		Runs: *n, Parallel: *parallel, MaxRepairs: *repairs, NoDocs: *noDocs,
		OutDir: *out, Verify: verifiers, Engine: eng, Log: os.Stdout,
	})
	return err
}

// staff is who does which job, as chosen on the command line.
type staff struct {
	reception, design, review, build, rescue, plan, judge *string
}

func staffFlags(fs *flag.FlagSet) staff {
	return staff{
		reception: fs.String("reception", workModel, "model that talks with the customer"),
		design:    fs.String("design", workModel, "model that writes the spec"),
		review:    fs.String("review", strongModel, "model that re-derives the spec's expected values, or none"),
		build:     fs.String("build", workModel, "model that writes main.go"),
		rescue:    fs.String("rescue", strongModel, "model for the last repair, or none"),
		plan:      fs.String("plan", workModel, "model that turns pairings into product ideas"),
		judge:     fs.String("judge", strongModel, "model that scores the ideas"),
	}
}

func (s staff) desk() order.Models {
	return order.Models{Reception: *s.reception, Design: *s.design, Review: orNone(*s.review)}
}

// floor is the configuration shared by every way of opening the factory.
func floor(s staff) (tui.Config, error) {
	eng, err := engine.NewClaude()
	if err != nil {
		return tui.Config{}, err
	}
	if _, err := exec.LookPath("go"); err != nil {
		return tui.Config{}, fmt.Errorf("the go command was not found in PATH")
	}
	dir, err := os.Getwd()
	if err != nil {
		return tui.Config{}, err
	}
	return tui.Config{
		Engine: eng, Dir: dir,
		Desk: s.desk(), Build: *s.build, Rescue: orNone(*s.rescue),
		Plan: *s.plan, Judge: *s.judge,
		Owner:  firstLine("gh", "api", "user", "-q", ".login"),
		Holder: firstLine("git", "config", "user.name"),
	}, nil
}

// cmdFloor opens the interactive factory in the current directory.
func cmdFloor(args []string) error {
	fs := flag.NewFlagSet("kojo", flag.ExitOnError)
	s := staffFlags(fs)
	fs.Parse(args)
	cfg, err := floor(s)
	if err != nil {
		return err
	}
	return tui.Run(cfg)
}

// cmdMix has the factory invent its own products. With -plain it prints
// progress instead of opening the floor.
func cmdMix(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("mix", flag.ExitOnError)
	s := staffFlags(fs)
	plain := fs.Bool("plain", false, "print progress instead of opening the floor")
	target := fs.Int("n", 3, "products to make before stopping")
	per := fs.Int("per", 0, "ideas sent to the line in each round (default 3, or 5 when -n is 10 or more)")
	top := fs.Int("top", 3, "how many to name at the end")
	draws := fs.Int("draws", 10, "pairings to draw in each round")
	fs.Parse(args)
	seed := strings.Join(fs.Args(), " ")
	if *per <= 0 {
		*per = 3
		if *target >= 10 {
			*per = 5
		}
	}

	cfg, err := floor(s)
	if err != nil {
		return err
	}
	if !*plain {
		cfg.Mix = &tui.MixOrder{Seed: seed, Target: *target, PerRound: *per, Top: *top, Draws: *draws}
		return tui.Run(cfg)
	}

	owner := cfg.Owner
	if owner == "" {
		owner = "local"
	}
	out, err := mix.Loop(ctx, mix.LoopConfig{
		Config: mix.Config{
			Engine: cfg.Engine, Plan: cfg.Plan, Judge: cfg.Judge, Desk: cfg.Desk, Build: cfg.Build, Rescue: cfg.Rescue,
			Dir: cfg.Dir, Owner: owner, Holder: cfg.Holder, Seed: seed, Draws: *draws,
			Rand: rand.New(rand.NewSource(time.Now().UnixNano())),
		},
		Target: *target, PerRound: min(*per, *target), Top: *top,
	}, func(e mix.Event) {
		switch e.Kind {
		case "round":
			fmt.Printf("\n第 %d 回（完成 %d / %d 本）\n", e.Round, e.Built, e.Target)
		case "pairs":
			for i, p := range e.Pairs {
				fmt.Printf("  くじ %d. %s × %s\n", i, p.Seed, p.Material)
			}
		case "planning":
			fmt.Println("  企画係が案を書いています…")
		case "judging":
			fmt.Printf("  選考係が %d 案を採点しています…\n", len(e.Ideas))
		case "picked":
			for _, i := range e.Ideas {
				mark := "  ボツ"
				if i.Picked {
					mark = "★ 採用"
				}
				fmt.Printf("  %s %-10s %2d点 (面白い%d 作れる%d 使う%d 新しい%d) %s — %s\n", mark, i.Name, i.Total(), i.Fun, i.Fit, i.Use, i.Fresh, i.Pitch, i.Reason)
			}
		case "product":
			if e.Stage == "end" {
				fmt.Printf("  [%s] %s %s\n", e.Result.Idea.Name, e.Result.Status, strings.Join(strings.Fields(e.Result.Reason), " "))
			}
		case "ranking":
			fmt.Printf("\n審査係が %d 本の完成品を見比べています…\n", e.Built)
		}
	})
	if len(out.Ranking) > 0 {
		fmt.Printf("\n完成 %d 本（ボツ %d 本、%d 回）。面白かった順:\n", len(out.Built), out.Dropped, out.Rounds)
		for n, r := range out.Ranking {
			mark := "  "
			if n < *top {
				mark = "★ "
			}
			fmt.Printf("%s%2d 位 %-10s %3d 点  %s\n      %s\n", mark, n+1, r.Name, r.Score, r.Result.Idea.Pitch, r.Comment)
		}
	}
	fmt.Printf("費用の目安: $%.2f\n", out.CostUSD)
	return err
}

func orNone(model string) string {
	if model == "none" {
		return ""
	}
	return model
}

// The two ways of staffing the factory that eval compares.
var arms = map[string]eval.Arm{
	// One model does everything and nobody checks the spec.
	"solo": {Name: "solo", Desk: order.Models{Reception: workModel, Design: workModel}, Build: workModel},
	// Each job goes to the model that suits it, and a second model checks the spec.
	"team": {Name: "team", Desk: order.Models{Reception: workModel, Design: workModel, Review: strongModel}, Build: workModel, Rescue: strongModel},
}

// cmdDemo runs the floor on a scripted engine, building into a scratch
// directory that is removed afterwards. With mix it replays a recorded run
// of the planning room: four tools in three rounds, then the jury.
func cmdDemo(planning bool) error {
	dir, err := os.MkdirTemp("", "kojo-demo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	team := arms["team"]
	cfg := tui.Config{
		Engine: &demo.Engine{
			Reception: 1500 * time.Millisecond, Design: 3500 * time.Millisecond, Review: 3 * time.Second, Build: 9 * time.Second,
			Plan: 4 * time.Second, Judge: 4 * time.Second, Jury: 5 * time.Second, Unattended: planning,
		},
		Desk: team.Desk, Build: team.Build, Rescue: team.Rescue, Plan: workModel, Judge: strongModel,
		Dir: dir, Holder: "go-kojo demo",
	}
	if planning {
		cfg.Mix = &tui.MixOrder{Target: 4, PerRound: 2, Top: 3}
	}
	return tui.Run(cfg)
}

func firstLine(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
}

func indent(s string) string {
	return "          " + strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n          ")
}
