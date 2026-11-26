package mix

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/dorako321/go-kojo/internal/engine"
	"github.com/dorako321/go-kojo/internal/order"
)

func TestDrawUsesTheSeedOrTheShelf(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, p := range Draw(r, "QR コード", nil, 8) {
		if p.Seed != "QR コード" || p.Material == "" {
			t.Errorf("pair %+v does not start from the user's seed", p)
		}
	}
	pairs := Draw(r, "", []Product{{"logspark", "counts log lines"}}, len(Seeds)+1)
	seen, materials := false, map[string]bool{}
	for _, p := range pairs {
		seen = seen || strings.HasPrefix(p.Seed, "logspark")
		materials[p.Material] = true
	}
	if !seen {
		t.Error("the shelf's product was never used as a seed")
	}
	if len(materials) != len(pairs) {
		t.Errorf("materials repeat within one draw: %d distinct in %d pairs", len(materials), len(pairs))
	}
}

func TestTotalFavoursFunAndBuildability(t *testing.T) {
	dullButUseful := Idea{Use: 5, Fit: 5, Fresh: 2, Fun: 1}
	funAndBuildable := Idea{Use: 2, Fit: 5, Fresh: 4, Fun: 5}
	funButImpossible := Idea{Use: 3, Fit: 1, Fresh: 5, Fun: 5}
	if !(funAndBuildable.Total() > dullButUseful.Total() && funAndBuildable.Total() > funButImpossible.Total()) {
		t.Errorf("totals: fun+buildable %d, dull %d, impossible %d", funAndBuildable.Total(), dullButUseful.Total(), funButImpossible.Total())
	}
}

// room plays every role with canned answers. Each planning round proposes
// tools that upper-case their input under new names; the builder fills the
// frame it is shown, so real programs are compiled and inspected.
type room struct {
	round int
	calls []string
}

var nameRe = regexp.MustCompile(`コマンド名は ([a-z0-9]+)`)

func (r *room) Call(_ context.Context, req engine.Request) (engine.Response, error) {
	reply := func(v any) (engine.Response, error) {
		b, _ := json.Marshal(v)
		return engine.Response{Output: b, CostUSD: 0.01}, nil
	}
	switch {
	case strings.Contains(req.Schema, `"pitch"`): // planner
		r.round++
		r.calls = append(r.calls, "plan")
		var ideas []map[string]any
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("kojomix%d%c", r.round, 'a'+i)
			ideas = append(ideas, map[string]any{"pair": i, "skip": false, "name": name, "pitch": name + " の案", "order": "大文字にする。コマンド名は " + name})
		}
		return reply(map[string]any{"ideas": ideas})
	case strings.Contains(req.Schema, `"fresh"`): // committee: later pairs score higher
		r.calls = append(r.calls, "judge")
		var scores []map[string]any
		for i := 0; i < 3; i++ {
			scores = append(scores, map[string]any{"pair": i, "use": 3, "fit": 5, "fresh": 3, "fun": 1 + i, "reason": "理由"})
		}
		return reply(map[string]any{"scores": scores})
	case strings.Contains(req.Schema, `"ranking"`): // jury: the alphabetically last name wins
		r.calls = append(r.calls, "jury")
		var ranking []map[string]any
		for n, m := range regexp.MustCompile(`(?m)^### (\S+)$`).FindAllStringSubmatch(req.Prompt, -1) {
			ranking = append(ranking, map[string]any{"name": m[1], "score": 10 + n*10, "comment": "講評"})
		}
		return reply(map[string]any{"ranking": ranking})
	case strings.Contains(req.Schema, `"brief"`): // reception
		return reply(map[string]any{"action": "handoff", "message": "", "options": []string{}, "brief": "upper-case"})
	case strings.Contains(req.Schema, `"main_go"`): // builder: fill the frame it was shown
		frame := req.Prompt[strings.Index(req.Prompt, "```go\n")+6:]
		frame = frame[:strings.Index(frame, "```")]
		stub := regexp.MustCompile(`(?s)\tfmt\.Fprintln\(stderr, "[a-z0-9]+: not implemented"\)\n\treturn 1\n`)
		src := stub.ReplaceAllString(frame, "\tb, _ := io.ReadAll(stdin)\n\tfmt.Fprint(stdout, strings.ToUpper(string(b)))\n\treturn 0\n")
		src = strings.Replace(src, "\t\"io\"\n", "\t\"io\"\n\t\"strings\"\n", 1)
		return reply(map[string]string{"status": "ok", "reason": "", "main_go": src})
	default: // designer
		name := nameRe.FindStringSubmatch(req.Prompt)[1]
		spec := map[string]any{
			"name": name, "summary": name + " prints its input in upper case.", "kind": "convert", "usage": name,
			"flags": []any{}, "behavior": []string{"B1: Copy stdin to stdout in upper case."}, "deps": []string{}, "network": false,
			"cases": []map[string]any{
				{"name": "hi", "args": []string{}, "stdin": "hi\n", "files": []any{}, "exit": 0, "stdout": map[string]string{"mode": "exact", "value": "HI\n"}, "stderr": map[string]string{"mode": "any", "value": ""}},
				{"name": "empty", "args": []string{}, "stdin": "", "files": []any{}, "exit": 0, "stdout": map[string]string{"mode": "exact", "value": ""}, "stderr": map[string]string{"mode": "any", "value": ""}},
			},
		}
		return reply(map[string]any{"action": "propose", "message": "作ります", "options": []string{}, "spec": spec})
	}
}

// The loop keeps going until it has enough products, feeds what it made
// back in as seeds, and names the jury's favourites.
func TestLoopBuildsUntilTargetAndRanks(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles real programs")
	}
	dir := t.TempDir()
	eng := &room{}
	var rounds []int
	seedsSeen := map[string]bool{}
	out, err := Loop(context.Background(), LoopConfig{
		Config: Config{
			Engine: eng, Plan: "work", Judge: "strong", Desk: order.Models{Reception: "work", Design: "work"},
			Build: "work", Dir: dir, Owner: "example", Holder: "tester", Draws: 3, Rand: rand.New(rand.NewSource(7)),
		},
		Target: 4, PerRound: 2, Top: 3,
	}, func(e Event) {
		if e.Kind == "round" {
			rounds = append(rounds, e.Built)
		}
		for _, p := range e.Pairs {
			seedsSeen[strings.SplitN(p.Seed, "（", 2)[0]] = true
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Built) != 4 || out.Rounds != 2 || fmt.Sprint(rounds) != "[0 2]" {
		t.Fatalf("built %d in %d rounds, round starts %v", len(out.Built), out.Rounds, rounds)
	}
	// Each round builds the two ideas the committee scored highest (b and c).
	var names []string
	for _, r := range out.Built {
		names = append(names, r.Report.Name)
		if _, err := os.Stat(filepath.Join(dir, r.Report.Name, r.Report.Name)); err != nil {
			t.Errorf("%s has no binary in its directory", r.Report.Name)
		}
	}
	for _, want := range []string{"kojomix1b", "kojomix1c", "kojomix2b", "kojomix2c"} {
		if !strings.Contains(strings.Join(names, " "), want) {
			t.Errorf("built %v, missing %s", names, want)
		}
	}
	// Round two could draw on what round one made.
	if !seedsSeen["kojomix1b"] && !seedsSeen["kojomix1c"] {
		t.Errorf("no first-round product was offered as a seed in round two: %v", seedsSeen)
	}
	// The jury's order, best first, with every built product in it.
	if len(out.Ranking) != 4 || out.Ranking[0].Score != 40 || out.Ranking[3].Score != 10 {
		t.Errorf("ranking %+v", out.Ranking)
	}
	if got := strings.Join(eng.calls, " "); got != "plan judge plan judge jury" {
		t.Errorf("calls %q", got)
	}
}
