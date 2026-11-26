package mix

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dorako321/go-kojo/internal/engine"
)

// LoopConfig keeps the planning room going round after round. Each round's
// products join the shelf and become seeds for the next, so later ideas are
// combinations of combinations.
type LoopConfig struct {
	Config
	Target   int // stop once this many products have passed inspection
	PerRound int // ideas sent to the line in each round
	Top      int // how many to name at the end
}

// Ranked is a finished product as the jury saw it.
type Ranked struct {
	Name    string
	Score   int // 1-100; zero when no jury sat
	Comment string
	Result  Result
}

// Outcome is everything a loop produced.
type Outcome struct {
	Built   []Result // products that passed inspection, in the order they were made
	Dropped int      // picked ideas that did not make it
	Rounds  int
	Ranking []Ranked // every built product, best first
	CostUSD float64
}

const juryRules = `You are the jury at a factory's exhibition. The tools in front of you were invented by forcing unrelated ideas together, and were then really built and inspected; each comes with an excerpt of its README, whose examples are real output.

Judge how interesting each one is. Reward a combination that is surprising and still makes you want to use it or show it to someone. Mark down the dull, the pointless, and near-duplicates of each other or of standard tools. Usefulness counts, but a useful tool that surprises nobody is not what this exhibition is for.

Give each tool a "score" from 1 to 100. Spread the scores out: the best and the worst should be far apart, and no two tools you find clearly different should share a score. Give a "comment": one Japanese sentence saying what makes it interesting, or what is missing.

Reply only through the JSON schema, one entry per tool, with the same names.`

const jurySchema = `{"type":"object","properties":{"ranking":{"type":"array","items":{"type":"object","properties":{` +
	`"name":{"type":"string"},"score":{"type":"integer"},"comment":{"type":"string"}},"required":["name","score","comment"]}}},"required":["ranking"]}`

// Loop runs rounds until Target products exist, then has a jury rank what
// was built. A round that builds nothing is tolerated once; twice in a row
// ends the loop with whatever there is.
func Loop(ctx context.Context, cfg LoopConfig, on func(Event)) (Outcome, error) {
	var out Outcome
	emit := func(e Event) {
		if on != nil {
			on(e)
		}
	}
	stalls := 0
	limit := (cfg.Target+cfg.PerRound-1)/cfg.PerRound + 3
	for round := 1; len(out.Built) < cfg.Target && round <= limit && ctx.Err() == nil; round++ {
		c := cfg.Config
		c.Count = min(cfg.PerRound, cfg.Target-len(out.Built))
		emit(Event{Kind: "round", Round: round, Built: len(out.Built), Target: cfg.Target})

		results, cost, err := Run(ctx, c, on)
		out.CostUSD += cost
		out.Rounds = round
		made := 0
		for _, r := range results {
			if r.Status == "ok" {
				out.Built = append(out.Built, r)
				made++
			} else {
				out.Dropped++
			}
		}
		if made > 0 {
			stalls = 0
		} else if stalls++; stalls == 2 {
			if len(out.Built) == 0 && err != nil {
				return out, err
			}
			break
		}
	}
	if len(out.Built) == 0 {
		return out, fmt.Errorf("no product passed inspection in %d rounds", out.Rounds)
	}

	for _, r := range out.Built {
		out.Ranking = append(out.Ranking, Ranked{Name: r.Report.Name, Result: r})
	}
	// With no more products than places there is nothing to rank.
	if len(out.Built) > cfg.Top && ctx.Err() == nil {
		emit(Event{Kind: "ranking", Built: len(out.Built)})
		cost, err := rank(ctx, cfg.Engine, cfg.Judge, out.Ranking)
		out.CostUSD += cost
		if err != nil {
			return out, fmt.Errorf("jury: %w", err)
		}
	}
	emit(Event{Kind: "ranked", Ranking: out.Ranking})
	return out, ctx.Err()
}

// rank asks the jury about every built product and sorts them by its score.
func rank(ctx context.Context, eng engine.Engine, model string, items []Ranked) (float64, error) {
	var b strings.Builder
	b.WriteString("## The tools\n")
	for _, it := range items {
		readme, _ := os.ReadFile(filepath.Join(it.Result.Report.Dir, "README.md"))
		text := string(readme)
		if i := strings.Index(text, "## License"); i > 0 {
			text = text[:i]
		}
		if len(text) > 1600 {
			text = text[:1600] + "\n…"
		}
		fmt.Fprintf(&b, "\n### %s\nIdea: %s\n\n%s\n", it.Name, it.Result.Idea.Pitch, strings.TrimSpace(text))
	}
	resp, err := eng.Call(ctx, engine.Request{System: juryRules, Prompt: b.String(), Schema: jurySchema, Model: model})
	if err != nil {
		return resp.CostUSD, err
	}
	var out struct {
		Ranking []struct {
			Name    string
			Score   int
			Comment string
		}
	}
	if err := json.Unmarshal(resp.Output, &out); err != nil {
		return resp.CostUSD, err
	}
	for _, r := range out.Ranking {
		for k := range items {
			if items[k].Name == r.Name {
				items[k].Score, items[k].Comment = max(1, min(100, r.Score)), r.Comment
			}
		}
	}
	sort.SliceStable(items, func(a, b int) bool { return items[a].Score > items[b].Score })
	return resp.CostUSD, nil
}
