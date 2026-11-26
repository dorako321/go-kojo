// Package order is the factory's front desk: it talks with the customer
// until the order is either a one-page spec the line can build, or a clear
// reason why it cannot be built.
package order

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dorako321/go-kojo/internal/engine"
	"github.com/dorako321/go-kojo/internal/spec"
)

// AllowedDeps are the only third-party modules a product may use. An order
// that needs anything else is turned down rather than fetched on a guess.
var AllowedDeps = []string{
	"github.com/skip2/go-qrcode",
	"github.com/mattn/go-runewidth",
	"github.com/mattn/go-isatty",
	"github.com/fatih/color",
	"github.com/google/uuid",
	"github.com/BurntSushi/toml",
	"gopkg.in/yaml.v3",
	"golang.org/x/term",
	"golang.org/x/text",
	"golang.org/x/net",
}

// Turn is one line of the conversation. Role is "customer", "desk" (what
// the factory said, including specs it proposed), "factory" (notes from the
// line, such as a failed inspection) or "review" (the second engineer).
type Turn struct {
	Role string
	Text string
}

// Models says which model does which job at the desk. The jobs differ in
// shape, so they do not all need the same model.
type Models struct {
	Reception string // talks with the customer: short replies, speed matters
	Design    string // writes the spec and its cases: long output
	Review    string // re-derives every expected value: short output, hard reasoning. Empty skips the review.
}

// Step is progress at the desk, for whoever is watching.
type Step struct {
	Role  string // reception | design | review
	Model string
	State string // start | done
	Note  string // on "done": ask | reject | handoff | sent back | revised | failed: ...

	// Set when a review is done.
	Reviewed int // cases the reviewer could work out
	Disputed int // of those, where it disagreed with the designer
}

// Call is one use of a model.
type Call struct {
	Role    string  `json:"role"`
	Model   string  `json:"model"`
	Seconds float64 `json:"seconds"`
	CostUSD float64 `json:"cost_usd"`
}

// Reply is the desk's next move.
type Reply struct {
	Action  string     // ask | propose | reject
	Message string     // what the desk says to the customer
	Options []string   // choices offered with a question or a rejection
	Spec    *spec.Spec // set when Action is "propose"
	CostUSD float64
	Calls   []Call

	Reviewed int      // cases the reviewer could work out
	Disputed []string // cases where reviewer and designer disagreed
	Revised  bool     // the designer rewrote the spec after the review
	Dispute  string   // the disagreement as it was put to the designer

	Questions int // set by Auto: questions answered without a person
}

type wireExpect struct {
	Mode  string `json:"mode"`
	Value string `json:"value"`
}

type wireCase struct {
	Name   string      `json:"name"`
	Args   []string    `json:"args"`
	Stdin  string      `json:"stdin"`
	Files  []spec.File `json:"files"`
	Exit   int         `json:"exit"`
	Stdout wireExpect  `json:"stdout"`
	Stderr wireExpect  `json:"stderr"`
}

type wireSpec struct {
	Name     string      `json:"name"`
	Summary  string      `json:"summary"`
	Kind     string      `json:"kind"`
	Usage    string      `json:"usage"`
	Flags    []spec.Flag `json:"flags"`
	Behavior []string    `json:"behavior"`
	Deps     []string    `json:"deps"`
	Network  bool        `json:"network"`
	Cases    []wireCase  `json:"cases"`
}

type wireReply struct {
	Action  string    `json:"action"`
	Message string    `json:"message"`
	Options []string  `json:"options"`
	Spec    *wireSpec `json:"spec"`
}

func (e wireExpect) expect() spec.Expect {
	switch e.Mode {
	case "exact":
		v := e.Value
		return spec.Expect{Exact: &v}
	case "contains":
		return spec.Expect{Contains: []string{e.Value}}
	case "regex":
		return spec.Expect{Regex: e.Value}
	}
	return spec.Expect{}
}

func (w wireSpec) spec() spec.Spec {
	s := spec.Spec{
		Name: w.Name, Summary: w.Summary, Kind: w.Kind, Usage: w.Usage,
		Flags: w.Flags, Behavior: w.Behavior, Deps: w.Deps, Network: w.Network,
	}
	for _, c := range w.Cases {
		s.Cases = append(s.Cases, spec.Case{
			Name: c.Name, Args: c.Args, Stdin: c.Stdin, Files: c.Files, Exit: c.Exit,
			Stdout: c.Stdout.expect(), Stderr: c.Stderr.expect(),
		})
	}
	return s
}

const expectSchema = `{"type":"object","properties":{"mode":{"type":"string","enum":["exact","contains","regex","any"]},"value":{"type":"string"}},"required":["mode","value"]}`

const specSchema = `{"type":"object","properties":{` +
	`"name":{"type":"string"},"summary":{"type":"string"},` +
	`"kind":{"type":"string","enum":["convert","inspect","generate","display","fetch"]},` +
	`"usage":{"type":"string"},` +
	`"flags":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"type":{"type":"string"},"default":{"type":"string"},"help":{"type":"string"}},"required":["name","type","default","help"]}},` +
	`"behavior":{"type":"array","items":{"type":"string"}},` +
	`"deps":{"type":"array","items":{"type":"string"}},` +
	`"network":{"type":"boolean"},` +
	`"cases":{"type":"array","items":{"type":"object","properties":{` +
	`"name":{"type":"string"},"args":{"type":"array","items":{"type":"string"}},"stdin":{"type":"string"},` +
	`"files":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"content":{"type":"string"}},"required":["name","content"]}},` +
	`"exit":{"type":"integer"},"stdout":` + expectSchema + `,"stderr":` + expectSchema +
	`},"required":["name","args","stdin","files","exit","stdout","stderr"]}}` +
	`},"required":["name","summary","kind","usage","flags","behavior","deps","network","cases"]}`

const receptionSchema = `{"type":"object","properties":{` +
	`"action":{"type":"string","enum":["ask","reject","handoff"]},` +
	`"message":{"type":"string"},` +
	`"options":{"type":"array","items":{"type":"string"}},` +
	`"brief":{"type":"string"}},"required":["action","message","options","brief"]}`

const designSchema = `{"type":"object","properties":{` +
	`"action":{"type":"string","enum":["propose","reject"]},` +
	`"message":{"type":"string"},` +
	`"options":{"type":"array","items":{"type":"string"}},` +
	`"spec":` + specSchema + `},"required":["action","message","options"]}`

const reviewSchema = `{"type":"object","properties":{"cases":{"type":"array","items":{"type":"object","properties":{` +
	`"name":{"type":"string"},"predictable":{"type":"boolean"},"exit":{"type":"integer"},"stdout":{"type":"string"},"note":{"type":"string"}},` +
	`"required":["name","predictable","exit","stdout","note"]}}},"required":["cases"]}`

// scope is what every role at the desk must know about the line.
var scope = `What the line can build:
- One main.go of at most 200 lines using the standard flag package: at most 3 flags, no subcommands.
- Input from arguments, from files named in arguments, and from stdin. Output to stdout and stderr. Exit code 0 on success, 1 on failure, 2 on usage error.
- Kinds: convert (a filter from input to output), inspect (the exit code says pass or fail), generate (IDs, passwords, sample data), display (draws something in the terminal), fetch (reads a public HTTP API that needs no login).
- Dependencies: the standard library whenever it is enough. Otherwise at most 3 modules, only from this list: ` + strings.Join(AllowedDeps, ", ") + `.
- Network access only for the fetch kind, and only after the customer has agreed to it.

What it cannot build: interactive or full-screen programs, anything that needs a login, an API key or other secret, servers and background services, GUIs, programs that delete or overwrite the user's files, programs that need other programs installed, suites of several commands, or anything that will not fit in 200 lines.`

var receptionRules = `You are the reception desk of a factory that builds one small, single-purpose command-line program in Go per order. You talk with the customer; a designer behind you writes the spec. Reply only through the JSON schema. Write "message" and "options" in the customer's language. Be brief and concrete; no greetings, no filler.

` + scope + `

Each turn, choose one action:
- "ask": one question about something that changes what gets built. Put 2 to 4 answers in "options", the one you recommend first. Ask at most 3 questions in the whole conversation, one per turn; for everything else let the designer assume a sensible default. If the program could reasonably work with or without the network, ask. Leave "brief" empty.
- "reject": when the line cannot build it. "message" says why in plain words. Put in "options" one or two smaller programs the line could build instead, phrased as orders the customer could place. Leave "brief" empty.
- "handoff": when a designer could write the spec from what you know. Put in "brief" one short paragraph in English saying exactly what to build and every decision made in this conversation, including whether the network may be used. Leave "message" and "options" empty.

Lines marked "desk" are what the factory said earlier, including specs it proposed. If the customer now wants a proposed spec changed, hand off with a brief that says what to change.`

var designRules = `You are the designer in a factory that builds one small, single-purpose command-line program in Go per order. You are given the conversation with the customer and a brief from reception, and you write the one-page spec the line builds from. Reply only through the JSON schema. Write "message" and "options" in the customer's language. Be brief and concrete.

` + scope + `

Choose one action:
- "propose": "message" is 2 to 4 short lines saying what the program will do and what you assumed. Fill "spec". Leave "options" empty.
- "reject": when it cannot be built inside the limits above. "message" says why in plain words. Put in "options" one or two smaller programs the line could build instead. Omit "spec".

Rules for the spec:
- name: the command name, 2 to 12 lowercase letters or digits, short to type.
- summary: one English sentence that starts with the name. usage: one line such as "name [-x] [file]". Neither may contain a backtick.
- flags: only the program's own flags. -h and -version are added by the factory; do not list them.
- behavior: numbered clauses "B1: ...", precise enough that two programmers would produce byte-identical output. Say what happens on empty input and on each kind of error, with the exact error text prefix "<name>: ".
- Text the program prints is English unless the customer asked otherwise.
- cases: 4 to 8 acceptance cases that are run against the finished program, covering the main path, an edge and an error. You cannot run the program, so work out every expected value from the behaviors with care. Use mode "exact" only when you are certain of every byte including the final newline; otherwise use "contains" or "regex"; use "any" with an empty value for a stream you do not check. Cases must not need the network or the current time, and must not depend on randomness.
- Files a case needs go in "files" with plain file names; the program runs in a directory that holds them.

Lines marked "factory" are notes from the production line, such as a spec it cannot take or an inspection that failed. If a note shows that one of your expected values was wrong, propose a corrected spec. If it shows the program cannot be built, reject and explain.

Lines marked "review" come from a second engineer who worked out the output of each case from your behaviors without seeing what you expect. Where the two of you differ, decide which is right by the behaviors alone. If the behaviors allow both readings, rewrite them so that only one is possible. Then make every case agree with the behaviors and propose the complete corrected spec.`

var reviewRules = `You are the second engineer in a factory that builds small command-line programs in Go. A designer has written a spec; you check it before anything is built. You are given the program's summary, usage, flags and numbered behaviors, and a list of cases with their arguments, stdin and files. You are not told what the designer expects each case to print.

For each case, work out from the behaviors alone exactly what a correct program writes to stdout, byte for byte including every newline, and its exit code. Go through the input step by step before you answer; do not guess.

House rules the program follows: results go to stdout; errors go to stderr as "<name>: <message>" with exit code 1 and nothing on stdout unless the behaviors say otherwise; a usage error exits 2; success exits 0. Flags are parsed by Go's standard flag package.

- If stdout cannot be worked out from the text, because it is random, depends on the clock, or is drawn by a library, set "predictable" to false and leave "stdout" empty.
- If the behaviors can be read in more than one way for a case, give the reading you find most natural and say in "note" what is ambiguous. Otherwise leave "note" empty.

Reply only through the JSON schema: one entry per case, with the same names, in the same order.`

type prediction struct {
	Name        string `json:"name"`
	Predictable bool   `json:"predictable"`
	Exit        int    `json:"exit"`
	Stdout      string `json:"stdout"`
	Note        string `json:"note"`
}

// Desk moves the order one step forward: reception talks with the
// customer, the designer writes the spec, and a reviewer on another model
// checks the designer's expected values before the line is asked to meet
// them. dir is where the product would be created. on may be nil.
func Desk(ctx context.Context, eng engine.Engine, m Models, turns []Turn, dir string, on func(Step)) (Reply, error) {
	var rep Reply
	step := func(role, model, state, note string) {
		if on != nil {
			on(Step{Role: role, Model: model, State: state, Note: note})
		}
	}
	call := func(role, model, system, prompt, schema string, out any) error {
		step(role, model, "start", "")
		resp, err := eng.Call(ctx, engine.Request{System: system, Prompt: prompt, Schema: schema, Model: model})
		rep.CostUSD += resp.CostUSD
		rep.Calls = append(rep.Calls, Call{Role: role, Model: model, Seconds: resp.Duration.Seconds(), CostUSD: resp.CostUSD})
		if err != nil {
			return fmt.Errorf("%s: %w", role, err)
		}
		if err := json.Unmarshal(resp.Output, out); err != nil {
			return fmt.Errorf("%s reply: %w", role, err)
		}
		return nil
	}

	// Reception, unless the line itself sent the order back: a failed
	// build is the designer's to answer, not something to ask the customer.
	brief := ""
	if len(turns) == 0 || turns[len(turns)-1].Role != "factory" {
		var r struct {
			Action, Message, Brief string
			Options                []string
		}
		if err := call("reception", m.Reception, receptionRules, transcript(turns, ""), receptionSchema, &r); err != nil {
			return rep, err
		}
		if r.Action != "handoff" {
			step("reception", m.Reception, "done", r.Action)
			rep.Action, rep.Message, rep.Options = r.Action, strings.TrimSpace(r.Message), r.Options
			return rep, nil
		}
		step("reception", m.Reception, "done", "handoff")
		brief = r.Brief
	}

	// Design. A spec the line cannot take goes back to the designer, not
	// to the customer, a limited number of times.
	design := func(extra []Turn) (wireReply, error) {
		var w wireReply
		err := call("design", m.Design, designRules, transcript(append(append([]Turn(nil), turns...), extra...), brief), designSchema, &w)
		if err == nil && w.Action == "propose" && w.Spec == nil {
			err = fmt.Errorf("design: proposed without a spec")
		}
		return w, err
	}
	var extra []Turn
	var sp spec.Spec
	var message string
	for try := 0; ; try++ {
		w, err := design(extra)
		if err != nil {
			return rep, err
		}
		if w.Action != "propose" {
			step("design", m.Design, "done", "reject")
			rep.Action, rep.Message, rep.Options = "reject", strings.TrimSpace(w.Message), w.Options
			return rep, nil
		}
		sp, message = w.Spec.spec(), strings.TrimSpace(w.Message)
		bad := Problems(sp, filepath.Join(dir, sp.Name))
		if len(bad) == 0 {
			break
		}
		if try == 2 {
			return rep, fmt.Errorf("the designer could not produce a spec the line accepts: %s", strings.Join(bad, "; "))
		}
		step("design", m.Design, "done", "sent back")
		extra = append(extra, Turn{"desk", "proposed spec: " + specJSON(sp)},
			Turn{"factory", "that spec cannot go to the line:\n- " + strings.Join(bad, "\n- ")})
	}
	step("design", m.Design, "done", fmt.Sprintf("%d cases", len(sp.Cases)))

	// Review. A failed review is reported but does not stop the order: the
	// inspection of the built program is still the final word.
	if m.Review != "" {
		var out struct{ Cases []prediction }
		if err := call("review", m.Review, reviewRules, reviewPrompt(sp), reviewSchema, &out); err != nil {
			step("review", m.Review, "done", "failed: "+err.Error())
		} else {
			disputes := compare(sp, out.Cases)
			for _, p := range out.Cases {
				if p.Predictable {
					rep.Reviewed++
				}
			}
			for _, d := range disputes {
				rep.Disputed = append(rep.Disputed, d.name)
			}
			if on != nil {
				on(Step{Role: "review", Model: m.Review, State: "done", Reviewed: rep.Reviewed, Disputed: len(disputes)})
			}
			if len(disputes) > 0 {
				rep.Dispute = disputeText(disputes)
				w, err := design(append(extra, Turn{"desk", "proposed spec: " + specJSON(sp)}, Turn{"review", disputeText(disputes)}))
				if err != nil {
					return rep, err
				}
				if w.Action != "propose" {
					step("design", m.Design, "done", "reject")
					rep.Action, rep.Message, rep.Options = "reject", strings.TrimSpace(w.Message), w.Options
					return rep, nil
				}
				// A revision the line cannot take is dropped; the spec as
				// first written stands and the inspection will judge it.
				if revised := w.Spec.spec(); len(Problems(revised, filepath.Join(dir, revised.Name))) == 0 {
					sp, message, rep.Revised = revised, strings.TrimSpace(w.Message), true
				}
				step("design", m.Design, "done", "revised")
			}
		}
	}

	rep.Action, rep.Message, rep.Spec = "propose", message, &sp
	return rep, nil
}

// Auto takes an order to a proposal or a rejection with nobody at the
// counter: a question from reception is answered with its first option,
// the one it recommends. The reply carries the cost and calls of every
// round, and Questions says how many were answered this way.
func Auto(ctx context.Context, eng engine.Engine, m Models, text, dir string, on func(Step)) (Reply, error) {
	var total Reply
	turns := []Turn{{Role: "customer", Text: text}}
	for {
		rep, err := Desk(ctx, eng, m, turns, dir, on)
		rep.CostUSD += total.CostUSD
		rep.Calls = append(total.Calls, rep.Calls...)
		rep.Questions = total.Questions
		if err != nil || rep.Action != "ask" {
			return rep, err
		}
		if rep.Questions++; rep.Questions > 3 {
			return rep, fmt.Errorf("reception kept asking")
		}
		answer := "おまかせします"
		if len(rep.Options) > 0 {
			answer = rep.Options[0]
		}
		turns = append(turns, Turn{"desk", rep.Message}, Turn{"customer", answer})
		total = rep
	}
}

func transcript(turns []Turn, brief string) string {
	var b strings.Builder
	b.WriteString("## Conversation so far\n\n")
	for _, t := range turns {
		fmt.Fprintf(&b, "%s: %s\n\n", t.Role, t.Text)
	}
	if brief != "" {
		fmt.Fprintf(&b, "## Brief from reception\n\n%s\n\n", brief)
	}
	b.WriteString("Reply with your next action.\n")
	return b.String()
}

func specJSON(s spec.Spec) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// reviewPrompt shows the reviewer the spec with every expected value
// removed, so its answers are its own.
func reviewPrompt(s spec.Spec) string {
	type blind struct {
		Name  string      `json:"name"`
		Args  []string    `json:"args"`
		Stdin string      `json:"stdin"`
		Files []spec.File `json:"files"`
	}
	view := struct {
		Name     string      `json:"name"`
		Summary  string      `json:"summary"`
		Usage    string      `json:"usage"`
		Flags    []spec.Flag `json:"flags"`
		Behavior []string    `json:"behavior"`
		Cases    []blind     `json:"cases"`
	}{Name: s.Name, Summary: s.Summary, Usage: s.Usage, Flags: s.Flags, Behavior: s.Behavior}
	for _, c := range s.Cases {
		view.Cases = append(view.Cases, blind{c.Name, c.Args, c.Stdin, c.Files})
	}
	b, _ := json.MarshalIndent(view, "", "  ")
	return "## Spec, without the expected results\n\n```json\n" + string(b) + "\n```\n\nWork out the result of every case.\n"
}

type dispute struct {
	name string
	c    spec.Case
	p    prediction
	why  []string
}

// compare holds the designer's expectations against what the reviewer
// derived. It is plain code: no model is asked which of the two is right.
func compare(s spec.Spec, preds []prediction) []dispute {
	byName := map[string]prediction{}
	for _, p := range preds {
		byName[p.Name] = p
	}
	var out []dispute
	for _, c := range s.Cases {
		p, ok := byName[c.Name]
		if !ok || !p.Predictable {
			continue
		}
		var why []string
		if p.Exit != c.Exit {
			why = append(why, fmt.Sprintf("exit code: you expect %d, the reviewer derives %d", c.Exit, p.Exit))
		}
		for _, w := range c.Stdout.Check(p.Stdout) {
			why = append(why, "stdout: you "+w+" (\"got\" is what the reviewer derives)")
		}
		if len(why) > 0 {
			out = append(out, dispute{c.Name, c, p, why})
		}
	}
	return out
}

func disputeText(ds []dispute) string {
	var b strings.Builder
	b.WriteString("a second engineer derived different results from your behaviors:\n")
	for _, d := range ds {
		fmt.Fprintf(&b, "\ncase %q (args %q, stdin %q)\n  the reviewer derives: exit %d, stdout %q\n", d.name, d.c.Args, d.c.Stdin, d.p.Exit, d.p.Stdout)
		for _, w := range d.why {
			fmt.Fprintf(&b, "  %s\n", w)
		}
		if d.p.Note != "" {
			fmt.Fprintf(&b, "  the reviewer notes: %s\n", d.p.Note)
		}
	}
	return b.String()
}

// Problems lists what stops a proposed spec from going to the line, in
// words the desk can act on. It returns nil when the spec may be built in dir.
func Problems(s spec.Spec, dir string) []string {
	var bad []string
	if err := s.Validate(); err != nil {
		bad = append(bad, err.Error())
	}
	for _, d := range s.Deps {
		ok := false
		for _, a := range AllowedDeps {
			if d == a || strings.HasPrefix(d, a+"/") {
				ok = true
			}
		}
		if !ok {
			bad = append(bad, fmt.Sprintf("dependency %s is not on the allowed list", d))
		}
	}
	if s.Network && s.Kind != "fetch" {
		bad = append(bad, "network is true but the kind is not fetch")
	}
	if path, err := exec.LookPath(s.Name); err == nil {
		bad = append(bad, fmt.Sprintf("the name %q is already a command on this machine (%s); choose another name", s.Name, path))
	}
	if _, err := os.Stat(dir); err == nil {
		bad = append(bad, fmt.Sprintf("a directory named %q already exists here; choose another name", s.Name))
	}
	return bad
}
