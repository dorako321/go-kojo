// Package mix is the factory's planning room. Nobody orders anything: it
// forces an existing idea together with an unrelated material, has a planner
// turn each pairing into a product idea, has a committee throw most of them
// out, and sends the survivors down the line unattended.
package mix

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/dorako321/go-kojo/internal/engine"
	"github.com/dorako321/go-kojo/internal/factory"
	"github.com/dorako321/go-kojo/internal/mold"
	"github.com/dorako321/go-kojo/internal/order"
	"github.com/dorako321/go-kojo/internal/ship"
	"github.com/dorako321/go-kojo/internal/spec"
)

type Config struct {
	Engine engine.Engine
	Plan   string       // writes many ideas: volume
	Judge  string       // scores them: a short, hard judgement
	Desk   order.Models // staff for each product, as for an ordinary order
	Build  string
	Rescue string
	Dir    string // products are created under this; its products are the shelf
	Owner  string
	Holder string
	Seed   string     // the user's own starting idea; empty draws from the shelf
	Count  int        // products to make
	Draws  int        // pairings to draw
	Rand   *rand.Rand // the lot
}

// Product is something already on the shelf.
type Product struct {
	Name    string
	Summary string
}

// Pair is one forced combination.
type Pair struct {
	Seed     string
	Material string
}

// Idea is what the planner made of a pair, and what the committee thought.
type Idea struct {
	Pair   int    `json:"pair"` // index into the pairs
	Skip   bool   `json:"skip"` // the planner found nothing honest to make
	Name   string `json:"name"`
	Pitch  string `json:"pitch"` // one line, for people
	Order  string `json:"order"` // the order placed with the line
	Use    int    `json:"use"`   // 1-5: would someone run it twice?
	Fit    int    `json:"fit"`   // 1-5: can the line build it?
	Fresh  int    `json:"fresh"` // 1-5: unlike the shelf and standard tools?
	Fun    int    `json:"fun"`   // 1-5: does it make you want to try it now?
	Reason string `json:"reason"`
	Picked bool   `json:"picked"`
}

// Total decides what gets built. The room exists to find things worth
// showing someone, so fun counts double; so does buildability, because an
// idea the line cannot make is worth nothing however good it sounds.
func (i Idea) Total() int { return 2*i.Fun + 2*i.Fit + i.Use + i.Fresh }

// Result is how one picked idea ended.
type Result struct {
	Idea    Idea
	Status  string // ok | rejected | failed | cannot | dispute | error
	Reason  string
	Spec    *spec.Spec
	Report  *factory.Report
	CostUSD float64
}

// Event is progress in the planning room and on the line.
type Event struct {
	Kind  string // round | pairs | planning | judging | picked | product | ranking | ranked
	Pairs []Pair
	Ideas []Idea

	// For the loop: which round is starting and how far along it is, and at
	// the end, every built product in the jury's order.
	Round, Built, Target int
	Ranking              []Ranked

	// For Kind "product": which picked idea, where it is, how it is going.
	Product int
	Stage   string // reception | design | review | build | repair | inspect | assemble | end
	State   string // start | ok | fail
	Text    string
	Source  string  // main.go, when a build or repair has just written it
	Result  *Result // set on stage "end"
}

// Shelf lists the products already made in dir.
func Shelf(dir string) []Product {
	var out []Product
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name(), "kojo.json"))
		if err != nil {
			continue
		}
		var m mold.Manifest
		if json.Unmarshal(b, &m) == nil && m.Spec.Name != "" {
			out = append(out, Product{m.Spec.Name, m.Spec.Summary})
		}
	}
	return out
}

// Draw makes n pairings. With a seed from the user every pairing starts from
// it; otherwise seeds come from the shelf and the built-in list.
func Draw(r *rand.Rand, seed string, shelf []Product, n int) []Pair {
	seeds := []string{seed}
	if seed == "" {
		// Alternate between what the factory has made and the stock ideas,
		// so its own products are always in the draw without crowding out
		// everything else as the shelf fills up.
		var own []string
		for _, p := range shelf {
			own = append(own, p.Name+"（"+p.Summary+"）")
		}
		stock := append([]string(nil), Seeds...)
		r.Shuffle(len(own), func(i, j int) { own[i], own[j] = own[j], own[i] })
		r.Shuffle(len(stock), func(i, j int) { stock[i], stock[j] = stock[j], stock[i] })
		seeds = nil
		for i := 0; i < max(len(own), len(stock)); i++ {
			if i < len(own) {
				seeds = append(seeds, own[i])
			}
			if i < len(stock) {
				seeds = append(seeds, stock[i])
			}
		}
	}
	lot := r.Perm(len(Materials))
	pairs := make([]Pair, n)
	for i := range pairs {
		pairs[i] = Pair{seeds[i%len(seeds)], Materials[lot[i%len(lot)]]}
	}
	return pairs
}

const limits = `The line builds one small command-line program in Go per order: a single main.go of at most 200 lines, the standard flag package with at most 3 flags and no subcommands, input from arguments, files and stdin, output to stdout. It must work offline, without logins or secrets, without other programs installed, and without data tables too large to write by hand. It cannot be interactive or full-screen.`

const planRules = `You are the planning desk of a factory that builds small, single-purpose command-line tools. Nobody has ordered anything. You are handed forced pairings of an existing tool idea and an unrelated material, drawn by lot. For each pairing, invent one new tool that really combines the two, does something neither does alone, and that someone would plausibly run more than once.

` + limits + `

For each pairing reply with:
- "pair": its number.
- "skip": true when the pairing gives nothing honest and useful inside the limits. Skipping is better than a gimmick. Leave the other fields empty.
- "name": the command name, 3 to 10 lowercase letters or digits, short to type, not the name of a well-known command.
- "pitch": one Japanese sentence of at most 40 characters saying what it does.
- "order": the order to place with the line, in Japanese, 2 to 4 sentences: what goes in, what comes out, one concrete example of input and output, and "コマンド名は <name>".

Do not propose something a standard Unix tool already does. Reply only through the JSON schema, one entry per pairing.`

const judgeRules = `You are the selection committee of a factory that builds small command-line tools. The planning desk has turned forced pairings into product ideas. Most forced combinations are gimmicks; your job is to say so. Score each idea from 1 to 5 on:
- "use": would a person run this more than once for a real task? 1 is a novelty, 5 is something people already do by hand.
- "fit": can the line build it? 1 is impossible within the limits, 5 is a clear filter with checkable output.
- "fresh": is it unlike what is already on the shelf and unlike standard Unix tools? 1 is a duplicate, 5 is something you have not seen.
- "fun": does the combination itself make you want to try it right now, or show it to someone? 1 is dull, 5 is a delight. A gimmick with no point is not fun.
Give "reason": one short Japanese sentence, the main thing for or against.

` + limits + `

Reply only through the JSON schema, one entry per idea, with the same pair numbers.`

const planSchema = `{"type":"object","properties":{"ideas":{"type":"array","items":{"type":"object","properties":{` +
	`"pair":{"type":"integer"},"skip":{"type":"boolean"},"name":{"type":"string"},"pitch":{"type":"string"},"order":{"type":"string"}},` +
	`"required":["pair","skip","name","pitch","order"]}}},"required":["ideas"]}`

const judgeSchema = `{"type":"object","properties":{"scores":{"type":"array","items":{"type":"object","properties":{` +
	`"pair":{"type":"integer"},"use":{"type":"integer"},"fit":{"type":"integer"},"fresh":{"type":"integer"},"fun":{"type":"integer"},"reason":{"type":"string"}},` +
	`"required":["pair","use","fit","fresh","fun","reason"]}}},"required":["scores"]}`

// Run draws, plans, selects and builds. on, if not nil, is called with
// progress; calls are serialised. The cost returned covers every model used.
func Run(ctx context.Context, cfg Config, on func(Event)) ([]Result, float64, error) {
	var mu sync.Mutex
	emit := func(e Event) {
		if on != nil {
			mu.Lock()
			defer mu.Unlock()
			on(e)
		}
	}
	cost := 0.0
	ask := func(model, system, prompt, schema string, out any) error {
		resp, err := cfg.Engine.Call(ctx, engine.Request{System: system, Prompt: prompt, Schema: schema, Model: model})
		cost += resp.CostUSD
		if err != nil {
			return err
		}
		return json.Unmarshal(resp.Output, out)
	}

	shelf := Shelf(cfg.Dir)
	pairs := Draw(cfg.Rand, cfg.Seed, shelf, cfg.Draws)
	emit(Event{Kind: "pairs", Pairs: pairs})

	// Plan: one idea per pairing, or an honest skip.
	emit(Event{Kind: "planning"})
	var b strings.Builder
	b.WriteString("## Pairings\n\n")
	for i, p := range pairs {
		fmt.Fprintf(&b, "%d. %s × %s\n", i, p.Seed, p.Material)
	}
	if len(shelf) > 0 {
		b.WriteString("\n## Already on the shelf\n\nDo not propose a near-duplicate of one of these, and do not reuse a name.\n\n")
		for _, p := range shelf {
			fmt.Fprintf(&b, "- %s: %s\n", p.Name, p.Summary)
		}
	}
	var planned struct{ Ideas []Idea }
	if err := ask(cfg.Plan, planRules, b.String(), planSchema, &planned); err != nil {
		return nil, cost, fmt.Errorf("planning: %w", err)
	}
	var ideas []Idea
	taken := map[string]bool{}
	for _, p := range shelf {
		taken[p.Name] = true
	}
	for _, i := range planned.Ideas {
		if i.Skip || i.Name == "" || i.Pair < 0 || i.Pair >= len(pairs) || taken[i.Name] {
			continue
		}
		taken[i.Name] = true
		ideas = append(ideas, i)
	}
	if len(ideas) == 0 {
		return nil, cost, fmt.Errorf("planning: no pairing gave a usable idea")
	}

	// Judge: scores come from a model, the choice is made by code.
	emit(Event{Kind: "judging", Ideas: ideas})
	b.Reset()
	b.WriteString("## On the shelf already\n\n")
	for _, p := range shelf {
		fmt.Fprintf(&b, "- %s: %s\n", p.Name, p.Summary)
	}
	if len(shelf) == 0 {
		b.WriteString("(nothing yet)\n")
	}
	b.WriteString("\n## Ideas\n\n")
	for _, i := range ideas {
		fmt.Fprintf(&b, "pair %d (%s × %s): %s — %s\n  order: %s\n\n", i.Pair, pairs[i.Pair].Seed, pairs[i.Pair].Material, i.Name, i.Pitch, i.Order)
	}
	var judged struct {
		Scores []struct {
			Pair, Use, Fit, Fresh, Fun int
			Reason                     string
		}
	}
	if err := ask(cfg.Judge, judgeRules, b.String(), judgeSchema, &judged); err != nil {
		return nil, cost, fmt.Errorf("selection: %w", err)
	}
	for _, s := range judged.Scores {
		for k := range ideas {
			if ideas[k].Pair == s.Pair {
				ideas[k].Use, ideas[k].Fit, ideas[k].Fresh, ideas[k].Fun, ideas[k].Reason = clamp(s.Use), clamp(s.Fit), clamp(s.Fresh), clamp(s.Fun), s.Reason
			}
		}
	}
	sort.SliceStable(ideas, func(a, b int) bool { return ideas[a].Total() > ideas[b].Total() })
	var picked []Idea
	for k := range ideas {
		// An idea the committee thinks cannot be built is not sent to the
		// line just to fill the quota.
		if len(picked) < cfg.Count && ideas[k].Fit >= 3 {
			ideas[k].Picked = true
			picked = append(picked, ideas[k])
		}
	}
	emit(Event{Kind: "picked", Ideas: ideas})
	if len(picked) == 0 {
		return nil, cost, fmt.Errorf("selection: the committee found nothing worth building")
	}

	// Build the survivors side by side.
	results := make([]Result, len(picked))
	var wg sync.WaitGroup
	for n, idea := range picked {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[n] = runOne(ctx, cfg, n, idea, emit)
			emit(Event{Kind: "product", Product: n, Stage: "end", State: map[bool]string{true: "ok", false: "fail"}[results[n].Status == "ok"], Result: &results[n]})
		}()
	}
	wg.Wait()
	for _, r := range results {
		cost += r.CostUSD
	}
	return results, cost, ctx.Err()
}

func clamp(v int) int { return max(1, min(5, v)) }

// runOne takes one idea through the desk and the line with nobody watching.
func runOne(ctx context.Context, cfg Config, n int, idea Idea, emit func(Event)) Result {
	res := Result{Idea: idea, Status: "error"}
	step := func(stage, state, text string) {
		emit(Event{Kind: "product", Product: n, Stage: stage, State: state, Text: text})
	}

	rep, err := order.Auto(ctx, cfg.Engine, cfg.Desk, idea.Order, cfg.Dir, func(s order.Step) {
		state := map[string]string{"start": "start", "done": "ok"}[s.State]
		text := s.Note
		if s.Role == "review" && s.State == "done" && s.Note == "" {
			text = fmt.Sprintf("%d of %d disputed", s.Disputed, s.Reviewed)
		}
		step(s.Role, state, text)
	})
	res.CostUSD = rep.CostUSD
	if err != nil {
		res.Reason = err.Error()
		return res
	}
	if rep.Action != "propose" {
		res.Status, res.Reason = "rejected", rep.Message
		return res
	}
	res.Spec = rep.Spec

	built, err := factory.Build(ctx, *rep.Spec, filepath.Join(cfg.Dir, rep.Spec.Name), factory.Options{
		Engine: cfg.Engine, Model: cfg.Build, Rescue: cfg.Rescue, Owner: cfg.Owner, Holder: cfg.Holder,
		Request: "mix: " + idea.Pitch + " / " + idea.Order, MaxRepairs: 2,
		OnEvent: func(e factory.Event) {
			if e.State == "gate" {
				return
			}
			emit(Event{Kind: "product", Product: n, Stage: e.Stage, State: e.State, Text: e.Text, Source: e.Source})
		},
	})
	res.CostUSD += built.CostUSD
	res.Report, res.Status, res.Reason = &built, built.Status, built.Reason
	if err != nil {
		res.Status = "error"
		if res.Reason == "" {
			res.Reason = err.Error()
		}
	}
	if res.Status == "ok" {
		if _, err := ship.CopyBinary(built.Binary, built.Dir, built.Name); err != nil {
			res.Reason = err.Error()
		}
	} else if built.Created {
		// Nothing that failed inspection stays on the shelf.
		os.RemoveAll(built.Dir)
	}
	return res
}
