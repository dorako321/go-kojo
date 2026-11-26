// Package tui is the factory floor the user sees: a conversation with the
// order desk on one side, and the line building the product on the other.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/dorako321/go-kojo/internal/engine"
	"github.com/dorako321/go-kojo/internal/factory"
	"github.com/dorako321/go-kojo/internal/inspect"
	"github.com/dorako321/go-kojo/internal/mix"
	"github.com/dorako321/go-kojo/internal/order"
	"github.com/dorako321/go-kojo/internal/ship"
	"github.com/dorako321/go-kojo/internal/spec"
)

type phase int

const (
	phChat       phase = iota // waiting for the customer to type
	phThinking                // the desk is working out its reply
	phSpec                    // a spec is on the table, waiting for approval
	phBuilding                // the line is running
	phDone                    // the product is in its directory
	phConfirm                 // asking before publishing
	phPublishing              // pushing to GitHub
	phPublished
	phMix   // the planning room is inventing and building products
	phShelf // the products it made are on the shelf
)

type chatLine struct {
	role string // customer | desk | note | alert | option
	text string
}

type lamp struct {
	name  string
	state int // stIdle until the gate reports, then stOK or stBad
}

// Config is what the floor needs from outside.
type Config struct {
	Engine engine.Engine
	Desk   order.Models // who receives, designs and reviews an order
	Build  string       // model that writes main.go
	Rescue string       // stronger model for the last repair; may be empty
	Plan   string       // model that turns pairings into product ideas
	Judge  string       // model that scores the ideas
	Mix    *MixOrder    // if set, the floor opens straight into the planning room
	Owner  string       // GitHub login; empty when gh is not logged in
	Holder string       // name for the LICENSE
	Dir    string       // products are created as subdirectories of this
}

// MixOrder asks the factory to invent products instead of taking an order.
type MixOrder struct {
	Seed     string // the user's own starting idea; empty draws from the shelf
	Target   int    // products to make before stopping
	PerRound int    // ideas sent to the line in each round
	Top      int    // how many to name at the end
	Draws    int    // pairings to draw in each round
}

// parseMix reads what follows "/mix": an optional number of products to
// make, then an optional idea to start from. "/mix 20 QRコード" is both.
func parseMix(rest string) MixOrder {
	var o MixOrder
	words := strings.Fields(rest)
	if len(words) > 0 {
		if n, err := strconv.Atoi(words[0]); err == nil && n > 0 && n <= 50 {
			o.Target, words = n, words[1:]
		}
	}
	o.Seed = strings.Join(words, " ")
	return o
}

// short turns a model ID into the word shown on screen.
func short(model string) string {
	m := strings.TrimPrefix(model, "claude-")
	if i := strings.Index(m, "-"); i > 0 {
		m = m[:i]
	}
	return m
}

// workers names who staffs each station, for the labels over the belt.
func (c Config) workers() [5]string {
	design := short(c.Desk.Design)
	if c.Desk.Review != "" {
		design += " + " + short(c.Desk.Review)
	}
	build := short(c.Build)
	if c.Rescue != "" && c.Rescue != c.Build {
		build += " → " + short(c.Rescue)
	}
	return [5]string{short(c.Desk.Reception), design, build, "go", "git gh"}
}

type Model struct {
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc

	w, h    int
	frame   int
	phase   phase
	input   textinput.Model
	chat    []chatLine
	turns   []order.Turn
	options []string
	sp      *spec.Spec
	working string       // who at the desk is busy, for the waiting line
	desk    chan tea.Msg // progress of the desk while it works
	maker   string       // model writing or repairing main.go right now
	mx      *mixState    // the planning room, while it is or was in use
	events  chan tea.Msg
	// inFlight receives the report of a build that was still running when
	// the user left, so its directory can be cleaned up.
	inFlight chan factory.Report

	line      line
	gates     []lamp
	attempt   int
	started   time.Time
	rep       *factory.Report
	sample    []string // a real run, shown on the shipping label
	plan      ship.Plan
	installed string
	repoURL   string

	shipped   int
	cost      float64
	celebrate int
}

type (
	tickMsg time.Time
	deskMsg struct {
		reply order.Reply
		err   error
	}
	stepMsg  order.Step
	eventMsg factory.Event
	builtMsg struct {
		rep factory.Report
		err error
	}
	planMsg struct {
		plan ship.Plan
		err  error
	}
	publishedMsg struct {
		url string
		err error
	}
)

var gateNames = []string{"imports", "lines", "gofmt", "deps", "build", "vet", "cases", "test"}

var gateHelp = map[string]string{
	"parse":   "Go として読めるか",
	"imports": "宣言した依存だけを使っているか",
	"lines":   "200 行以内か",
	"gofmt":   "整形されているか",
	"deps":    "余計なモジュールを引いていないか",
	"build":   "コンパイルできるか",
	"vet":     "go vet の指摘がないか",
	"cases":   "受け入れケースを実機で実行",
	"test":    "同じケースを go test でも実行",
}

func New(cfg Config) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	in := textinput.New()
	in.Prompt = "▸ "
	in.Placeholder = "欲しい CLI を一言で（例: CSV を Markdown の表にしたい）"
	st := textinput.DefaultDarkStyles()
	st.Focused.Prompt = lipgloss.NewStyle().Foreground(cCyan.lip()).Bold(true)
	st.Focused.Text = lipgloss.NewStyle().Foreground(cInk.lip())
	st.Focused.Placeholder = lipgloss.NewStyle().Foreground(cFaint.lip())
	st.Cursor.Color = cPink.lip()
	in.SetStyles(st)
	in.CharLimit = 400
	// The terminal's own cursor is used, placed on the input by View. An
	// input method draws the text being composed at that cursor, so with a
	// painted cursor it would appear in the wrong place.
	in.SetVirtualCursor(false)

	m := &Model{cfg: cfg, ctx: ctx, cancel: cancel, input: in, line: newLine()}
	m.line.workers = cfg.workers()
	m.say("desk", "いらっしゃいませ。どんな道具が欲しいですか？ 一言でどうぞ。")
	return m
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tick(), m.input.Focus()}
	if m.cfg.Mix != nil {
		cmds = append(cmds, m.startMix(*m.cfg.Mix))
	}
	return tea.Batch(cmds...)
}

func tick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) say(role, text string) { m.chat = append(m.chat, chatLine{role, text}) }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.line.layout(m.w)
		m.input.SetWidth(max(10, m.w*55/100-8))
		return m, nil
	case tickMsg:
		m.frame++
		m.line.tick()
		if m.celebrate > 0 {
			m.celebrate--
		}
		return m, tick()
	case tea.KeyPressMsg:
		return m.key(msg)
	case mixMsg:
		m.onMix(mix.Event(msg))
		return m, wait(m.mx.ch)
	case mixEndMsg:
		m.onMixEnd(msg)
		return m, nil
	case stepMsg:
		m.onStep(order.Step(msg))
		return m, wait(m.desk)
	case deskMsg:
		m.desk = nil
		return m, m.onDesk(msg)
	case eventMsg:
		m.onEvent(factory.Event(msg))
		return m, wait(m.events)
	case builtMsg:
		return m, m.onBuilt(msg)
	case planMsg:
		if msg.err != nil {
			m.say("alert", "公開できません: "+msg.err.Error())
			return m, nil
		}
		m.plan, m.phase = msg.plan, phConfirm
		return m, nil
	case publishedMsg:
		if msg.err != nil {
			m.say("alert", "公開に失敗しました: "+msg.err.Error())
			m.line.st[sShip] = stBad
			m.phase = phDone
			return m, nil
		}
		m.repoURL, m.phase, m.celebrate = msg.url, phPublished, 100
		m.line.st[sShip] = stOK
		m.say("note", "公開しました: "+msg.url)
		return m, nil
	}
	if m.typing() {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) typing() bool { return m.phase == phChat || m.phase == phSpec }

func (m *Model) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if s == "ctrl+c" {
		m.cancel()
		return m, tea.Quit
	}
	switch m.phase {
	case phDone, phPublished:
		switch s {
		case "q":
			m.cancel()
			return m, tea.Quit
		case "n":
			m.reset()
		case "b":
			if m.mx != nil {
				m.phase = phShelf
			}
		case "i":
			m.install()
		case "p":
			if m.phase == phDone {
				return m, m.prepare()
			}
		}
		return m, nil
	case phShelf:
		switch {
		case s == "q":
			m.cancel()
			return m, tea.Quit
		case s == "n":
			m.reset()
		case s == "m":
			return m, m.startMix(m.mx.order)
		case len(s) == 1 && s[0] >= '1' && s[0] <= '9':
			m.openShelf(int(s[0] - '1'))
		}
		return m, nil
	case phMix:
		return m, nil
	case phConfirm:
		switch s {
		case "y":
			m.phase = phPublishing
			m.line.st[sShip] = stBusy
			return m, m.publish()
		case "n", "esc":
			m.phase = phDone
		}
		return m, nil
	case phThinking, phBuilding, phPublishing:
		return m, nil
	}

	switch s {
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			if m.phase == phSpec {
				return m, m.build()
			}
			return m, nil
		}
		m.input.Reset()
		// "/mix" hands the floor to the planning room; anything after it
		// is the idea to start from.
		if rest, ok := strings.CutPrefix(text, "/mix"); ok {
			return m, m.startMix(parseMix(rest))
		}
		return m, m.send(text)
	case "esc":
		m.input.Reset()
		return m, nil
	}
	// With nothing typed, a digit picks one of the offered answers.
	if m.input.Value() == "" && len(s) == 1 && s[0] >= '1' && int(s[0]-'0') <= len(m.options) {
		return m, m.send(m.options[s[0]-'1'])
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}

// send passes what the customer said to the desk.
func (m *Model) send(text string) tea.Cmd {
	m.say("customer", text)
	m.turns = append(m.turns, order.Turn{Role: "customer", Text: text})
	m.options = nil
	m.line.enqueue(crTicket, "注文")
	return m.ask()
}

// ask puts the order in front of the desk and follows its progress.
func (m *Model) ask() tea.Cmd {
	m.phase = phThinking
	m.working = ""
	turns := append([]order.Turn(nil), m.turns...)
	ch := make(chan tea.Msg, 16)
	m.desk = ch
	go func() {
		r, err := order.Desk(m.ctx, m.cfg.Engine, m.cfg.Desk, turns, m.cfg.Dir, func(s order.Step) { ch <- stepMsg(s) })
		ch <- deskMsg{r, err}
	}()
	return wait(ch)
}

// onStep shows who at the desk has the order now.
func (m *Model) onStep(s order.Step) {
	l := &m.line
	who := map[string]string{"reception": "受付", "design": "設計係", "review": "検図係"}[s.Role] + "（" + short(s.Model) + "）"
	if s.State == "start" {
		m.working = who + map[string]string{"reception": "が聞き取っています", "design": "が仕様を書いています", "review": "が期待値を検算しています"}[s.Role]
		switch s.Role {
		case "reception":
			l.st[sOrder] = stBusy
		default:
			l.st[sOrder], l.st[sDesign] = stOK, stBusy
		}
		return
	}
	switch {
	case s.Role == "reception" && s.Note == "handoff":
		m.say("note", who+" → 設計係に回しました")
	case s.Role == "design" && s.Note == "sent back":
		m.say("note", who+"の仕様がラインの規格に合わず、書き直しています")
	case s.Role == "design" && s.Note == "revised":
		m.say("note", who+"が仕様を直しました")
	case s.Role == "review" && strings.HasPrefix(s.Note, "failed"):
		m.say("note", who+"の検算は行えませんでした。そのまま進めます")
	case s.Role == "review" && s.Disputed == 0:
		m.say("note", fmt.Sprintf("%s: %d ケースを検算、食い違いなし", who, s.Reviewed))
	case s.Role == "review":
		m.say("alert", fmt.Sprintf("%s: %d ケース中 %d 件が設計と食い違い → 設計係に戻します", who, s.Reviewed, s.Disputed))
	}
}

func (m *Model) onDesk(msg deskMsg) tea.Cmd {
	m.cost += msg.reply.CostUSD
	m.phase = phChat
	m.line.st[sOrder] = stIdle
	if msg.err != nil {
		m.say("alert", "受付で問題が起きました: "+msg.err.Error())
		return nil
	}
	r := msg.reply
	switch r.Action {
	case "propose":
		m.sp = r.Spec
		js, _ := json.Marshal(r.Spec)
		m.turns = append(m.turns, order.Turn{Role: "desk", Text: r.Message + "\nproposed spec: " + string(js)})
		m.say("desk", r.Message)
		m.phase = phSpec
		m.rep = nil
		m.line = m.freshLine()
		m.line.st[sOrder], m.line.st[sDesign] = stOK, stBusy
		m.line.spawnAt, m.line.hold = sOrder, sDesign
		m.line.enqueue(crPart, partLabels(*r.Spec)...)
	case "reject":
		m.turns = append(m.turns, order.Turn{Role: "desk", Text: r.Message})
		m.say("alert", "この注文は作れません。")
		m.say("desk", r.Message)
		m.offer(r.Options)
		m.line.st[sOrder] = stBad
	default:
		m.turns = append(m.turns, order.Turn{Role: "desk", Text: r.Message})
		m.say("desk", r.Message)
		m.offer(r.Options)
	}
	return nil
}

func (m *Model) offer(options []string) {
	if len(options) > 4 {
		options = options[:4]
	}
	m.options = options
	for i, o := range options {
		m.say("option", fmt.Sprintf("%d  %s", i+1, o))
	}
}

func (m *Model) freshLine() line {
	l := newLine()
	l.layout(m.w)
	l.workers = m.line.workers
	l.travel = m.line.travel
	return l
}

func partLabels(s spec.Spec) []string {
	var out []string
	for i := range s.Behavior {
		out = append(out, fmt.Sprintf("B%d", i+1))
	}
	return out
}

// build sends the approved spec down the line.
func (m *Model) build() tea.Cmd {
	sp := *m.sp
	m.phase = phBuilding
	m.started = time.Now()
	m.attempt = 0
	m.resetGates()
	m.say("note", "製造を始めます: "+sp.Name)
	m.line.st[sDesign] = stOK
	m.line.hold, m.line.absorb = -1, sMake
	m.line.spawnAt, m.line.loop = sDesign, partLabels(sp)

	ch := make(chan tea.Msg, 64)
	m.events = ch
	opt := factory.Options{
		Engine: m.cfg.Engine, Model: m.cfg.Build, Rescue: m.cfg.Rescue, Owner: m.owner(), Holder: m.cfg.Holder,
		Request: m.request(), MaxRepairs: 2,
		OnEvent: func(e factory.Event) { ch <- eventMsg(e) },
	}
	dir := filepath.Join(m.cfg.Dir, sp.Name)
	flight := make(chan factory.Report, 1)
	m.inFlight = flight
	go func() {
		rep, err := factory.Build(m.ctx, sp, dir, opt)
		flight <- rep
		ch <- builtMsg{rep, err}
	}()
	return wait(ch)
}

func wait(ch chan tea.Msg) tea.Cmd { return func() tea.Msg { return <-ch } }

func (m *Model) owner() string {
	if m.cfg.Owner != "" {
		return m.cfg.Owner
	}
	return "local"
}

// request is everything the customer typed, kept with the product.
func (m *Model) request() string {
	var said []string
	for _, t := range m.turns {
		if t.Role == "customer" {
			said = append(said, t.Text)
		}
	}
	return strings.Join(said, " / ")
}

func (m *Model) resetGates() {
	m.gates = m.gates[:0]
	for _, n := range gateNames {
		m.gates = append(m.gates, lamp{name: n})
	}
}

// onEvent turns progress on the real line into what the belt shows.
func (m *Model) onEvent(e factory.Event) {
	l := &m.line
	switch {
	case e.Stage == factory.StageBuild || e.Stage == factory.StageRepair:
		switch e.State {
		case "start":
			m.attempt++
			m.maker = e.Text
			if m.attempt > 1 {
				m.resetGates()
				note := fmt.Sprintf("手直し %d 回目（%s）", m.attempt-1, short(m.maker))
				if m.maker != m.cfg.Build {
					note += "。上位モデルに交代しました"
				}
				m.say("note", note)
			}
			l.st[sMake], l.st[sInspect] = stBusy, stIdle
			l.verdict, l.hold, l.absorb = mkNone, -1, sMake
			l.spawnAt, l.loop = sDesign, partLabels(*m.sp)
		case "ok":
			// The program exists now: its functions come out of the
			// machine and queue for inspection.
			l.st[sMake] = stOK
			l.loop, l.queue = nil, nil
			l.spawnAt, l.hold = sMake, sInspect
			l.enqueue(crFunc, funcNames(e.Source)...)
		case "fail":
			l.st[sMake] = stBad
			l.loop = nil
		}
	case e.Stage == factory.StageInspect:
		switch e.State {
		case "start":
			l.st[sInspect] = stBusy
		case "gate":
			m.setGate(e.Gate)
		case "ok":
			l.st[sInspect] = stOK
			l.verdict, l.hold = mkOK, -1
		case "fail":
			l.st[sInspect] = stBad
			l.reject()
		}
	case e.Stage == factory.StageAssemble:
		l.st[sShip] = map[string]int{"start": stBusy, "ok": stOK, "fail": stBad}[e.State]
	}
}

func (m *Model) setGate(g *inspect.Gate) {
	state := stBad
	if g.OK {
		state = stOK
	}
	for i := range m.gates {
		if m.gates[i].name == g.Name {
			m.gates[i].state = state
			return
		}
	}
	m.gates = append(m.gates, lamp{g.Name, state})
}

func funcNames(src string) []string {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", src, parser.SkipObjectResolution)
	if err != nil {
		return []string{"main.go"}
	}
	var names []string
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			name := fd.Name.Name
			if len(name) > 14 {
				name = name[:13] + "…"
			}
			names = append(names, name+"()")
		}
	}
	return names
}

func (m *Model) onBuilt(msg builtMsg) tea.Cmd {
	rep := msg.rep
	m.cost += rep.CostUSD
	m.events, m.inFlight = nil, nil
	m.line.loop = nil

	if rep.Status == "ok" {
		if _, err := ship.CopyBinary(rep.Binary, rep.Dir, rep.Name); err != nil {
			m.say("alert", "バイナリを置けませんでした: "+err.Error())
		}
		m.rep, m.phase = &rep, phDone
		m.shipped++
		m.celebrate = 100
		m.installed, m.repoURL = "", ""
		m.sample = m.sampleRun(rep)
		m.say("note", fmt.Sprintf("検品に合格しました。./%s/ に置いてあります。", rep.Name))
		return nil
	}

	// A product that failed inspection never stays on disk. Only a
	// directory Build created itself is removed.
	if rep.Created {
		os.RemoveAll(rep.Dir)
	}
	m.line.st[sShip] = stIdle
	why := rep.Reason
	if msg.err != nil && why == "" {
		why = msg.err.Error()
	}
	detail := why
	if n := len(rep.Attempts); n > 0 {
		for _, g := range rep.Attempts[n-1].Gates {
			if !g.OK {
				detail += "\n[" + g.Name + "] " + g.Detail
			}
		}
	}
	if len(detail) > 2500 {
		detail = detail[:2500] + "…"
	}
	switch rep.Status {
	case "cannot":
		m.say("alert", "製造できませんでした。")
	case "dispute":
		m.say("alert", "仕様とケースが食い違っていました。")
	case "failed":
		m.say("alert", fmt.Sprintf("検品に合格できませんでした（手直し %d 回）。出荷はしていません。", rep.Repairs()))
	default:
		m.say("alert", "ラインが止まりました: "+why)
		m.phase = phChat
		return nil
	}
	// The desk explains the failure in the customer's words, and proposes
	// a corrected spec when its own expectations were the problem.
	m.turns = append(m.turns, order.Turn{Role: "factory", Text: "the build ended with status " + rep.Status + ":\n" + detail})
	return m.ask()
}

// sampleRun executes the product once on one of its own cases so the label
// shows output that really came from it.
func (m *Model) sampleRun(rep factory.Report) []string {
	for _, c := range m.sp.Cases {
		if c.Exit != 0 || len(c.Files) > 0 {
			continue
		}
		out, err := inspect.Execute(m.ctx, rep.Binary, c)
		if err != nil || strings.TrimSpace(out.Stdout) == "" {
			continue
		}
		cmd := m.sp.Name + " " + strings.Join(c.Args, " ")
		if c.Stdin != "" {
			cmd = fmt.Sprintf("printf %q | %s", c.Stdin, cmd)
		}
		lines := []string{"$ " + strings.TrimSpace(cmd)}
		if strings.ContainsRune(out.Stdout, '\x1b') {
			return append(lines, "（端末に描画する出力です。実行して確かめてください）")
		}
		return append(lines, strings.Split(strings.TrimRight(out.Stdout, "\n"), "\n")...)
	}
	return nil
}

func (m *Model) install() {
	if m.rep == nil {
		return
	}
	path, err := ship.Install(m.rep.Binary, m.rep.Name)
	if err != nil {
		m.say("alert", "PATH に置けませんでした: "+err.Error())
		return
	}
	m.installed = path
	m.say("note", "PATH に置きました: "+path)
}

func (m *Model) prepare() tea.Cmd {
	if m.cfg.Owner == "" {
		m.say("alert", "公開には gh のログインが必要です（gh auth login）。")
		return nil
	}
	dir, name := m.rep.Dir, m.rep.Name
	return func() tea.Msg {
		plan, err := ship.Prepare(m.ctx, dir, m.cfg.Owner, name)
		return planMsg{plan, err}
	}
}

func (m *Model) publish() tea.Cmd {
	dir, name, summary := m.rep.Dir, m.rep.Name, m.sp.Summary
	return func() tea.Msg {
		url, err := ship.Publish(m.ctx, dir, m.cfg.Owner, name, summary)
		return publishedMsg{url, err}
	}
}

// reset clears the desk for the next order; the day's tally stays.
func (m *Model) reset() {
	m.phase = phChat
	m.turns, m.options, m.sp, m.rep, m.sample, m.mx = nil, nil, nil, nil, nil, nil
	m.line = m.freshLine()
	m.chat = nil
	m.say("desk", "次は何を作りましょう？")
	m.input.Reset()
}
