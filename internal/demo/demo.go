// Package demo is an engine that answers from a script instead of a model,
// so the whole floor can be watched, and tested, without spending anything.
// Everything after the engine is real: the files are written, compiled and
// inspected like any other product.
package demo

import (
	"context"
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/dorako321/go-kojo/internal/engine"
)

//go:embed propose.json
var propose []byte

//go:embed csv2md.go.txt
var source string

// mix.json is a recording of one real run of the planning room: the ideas
// the planner wrote, the scores the committee and the jury gave, and the
// specs and programs of the four tools that were built.
//
//go:embed mix.json
var mixJSON []byte

type recording struct {
	Rounds [][]struct {
		Name, Pitch, Order, Reason string
		Fun, Fit, Use, Fresh       int
	}
	Products map[string]struct {
		Propose json.RawMessage
		Source  string
	}
	Jury json.RawMessage
}

var mix = func() (r recording) {
	if err := json.Unmarshal(mixJSON, &r); err != nil {
		panic(err)
	}
	return r
}()

const (
	ask     = `{"action":"ask","message":"入力はどこから受けますか？","options":["ファイルと標準入力の両方（おすすめ）","標準入力だけ"],"brief":""}`
	handoff = `{"action":"handoff","message":"","options":[],"brief":"Build what the customer described."}`
)

// Engine replays recorded answers for every role. A single order gets a
// question from reception, a proposal for csv2md, a reviewer who disagrees
// about one case, and the program. The planning room gets its recorded
// rounds, one per planning call.
type Engine struct {
	Reception, Design, Review, Build time.Duration // how long each role appears to work
	Plan, Judge, Jury                time.Duration
	Unattended                       bool // reception hands off at once, as in the planning room
	asked                            bool
	round                            int
}

var (
	orderedName = regexp.MustCompile(`コマンド名は ([a-z0-9]+)`)
	specName    = regexp.MustCompile(`"name": "([a-z0-9]+)"`)
)

func (e *Engine) Call(ctx context.Context, req engine.Request) (engine.Response, error) {
	var out []byte
	var wait time.Duration
	switch {
	case strings.Contains(req.Schema, `"pitch"`):
		wait, out = e.Plan, e.plan()
	case strings.Contains(req.Schema, `"fresh"`):
		wait, out = e.Judge, e.scores()
	case strings.Contains(req.Schema, `"ranking"`):
		wait, out = e.Jury, []byte(`{"ranking":`+string(mix.Jury)+`}`)
	case strings.Contains(req.Schema, `"main_go"`):
		wait = e.Build
		src := source
		if m := specName.FindStringSubmatch(req.Prompt); m != nil && mix.Products[m[1]].Source != "" {
			src = mix.Products[m[1]].Source
		}
		out, _ = json.Marshal(map[string]string{"status": "ok", "reason": "", "main_go": src})
	case strings.Contains(req.Schema, `"predictable"`):
		wait, out = e.Review, review(req.Prompt)
	case strings.Contains(req.Schema, `"brief"`):
		wait, out = e.Reception, []byte(handoff)
		if !e.asked && !e.Unattended {
			e.asked, out = true, []byte(ask)
		}
	default:
		wait, out = e.Design, propose
		if m := orderedName.FindStringSubmatch(req.Prompt); m != nil && len(mix.Products[m[1]].Propose) > 0 {
			out = mix.Products[m[1]].Propose
		}
	}
	select {
	case <-time.After(wait):
	case <-ctx.Done():
		return engine.Response{}, ctx.Err()
	}
	return engine.Response{Output: out, Duration: wait}, nil
}

// plan returns the ideas recorded for the next round. Past the end of the
// recording it has nothing left to propose.
func (e *Engine) plan() []byte {
	var ideas []map[string]any
	if e.round < len(mix.Rounds) {
		for i, idea := range mix.Rounds[e.round] {
			ideas = append(ideas, map[string]any{"pair": i, "skip": false, "name": idea.Name, "pitch": idea.Pitch, "order": idea.Order})
		}
	}
	e.round++
	b, _ := json.Marshal(map[string]any{"ideas": ideas})
	return b
}

// scores returns what the committee said about the round just planned.
func (e *Engine) scores() []byte {
	var scores []map[string]any
	if n := e.round - 1; n >= 0 && n < len(mix.Rounds) {
		for i, idea := range mix.Rounds[n] {
			scores = append(scores, map[string]any{"pair": i, "fun": idea.Fun, "fit": idea.Fit, "use": idea.Use, "fresh": idea.Fresh, "reason": idea.Reason})
		}
	}
	b, _ := json.Marshal(map[string]any{"scores": scores})
	return b
}

// review answers as the second engineer. For csv2md it agrees with the
// designer on every case but one, where it reads the quoting rule
// differently; for anything else it has no objection.
func review(prompt string) []byte {
	if !strings.Contains(prompt, `"name": "csv2md"`) {
		return []byte(`{"cases":[]}`)
	}
	var p struct {
		Spec struct {
			Cases []struct {
				Name   string
				Exit   int
				Stdout struct{ Mode, Value string }
			}
		}
	}
	json.Unmarshal(propose, &p)
	type pred struct {
		Name        string `json:"name"`
		Predictable bool   `json:"predictable"`
		Exit        int    `json:"exit"`
		Stdout      string `json:"stdout"`
		Note        string `json:"note"`
	}
	var cases []pred
	for _, c := range p.Spec.Cases {
		out := pred{Name: c.Name, Exit: c.Exit, Predictable: c.Stdout.Mode == "exact", Stdout: c.Stdout.Value}
		if c.Name == "quoted" {
			out.Stdout = strings.Replace(out.Stdout, "| a,b |", `| "a,b" |`, 1)
			out.Note = "B2 does not say whether the quotes around a field are kept."
		}
		cases = append(cases, out)
	}
	b, _ := json.Marshal(map[string]any{"cases": cases})
	return b
}
