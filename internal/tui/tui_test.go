package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dorako321/go-kojo/internal/factory"
	"github.com/dorako321/go-kojo/internal/inspect"
	"github.com/dorako321/go-kojo/internal/mix"
	"github.com/dorako321/go-kojo/internal/order"
	"github.com/dorako321/go-kojo/internal/spec"
)

func testSpec() *spec.Spec {
	exact := "ok\n"
	return &spec.Spec{
		Name: "demo", Summary: "demo does a thing.", Kind: "convert", Usage: "demo [file]",
		Behavior: []string{"B1: one", "B2: two", "B3: 日本語の長い振る舞いの説明がここに入ります"},
		Cases: []spec.Case{
			{Name: "a", Exit: 0, Stdout: spec.Expect{Exact: &exact}},
			{Name: "b", Exit: 1},
		},
	}
}

// Every phase must fill the window exactly: a line that is one column too
// wide wraps in a real terminal and tears the whole screen.
func TestRenderFillsWindow(t *testing.T) {
	phases := map[string]func(m *Model){
		"chat": func(m *Model) {
			m.say("customer", "CSV を Markdown の表にしたい。できれば TSV も読めるとうれしいです、という長めの注文")
			m.say("desk", "入力はどこから受けますか？")
			m.offer([]string{"ファイルと標準入力の両方（おすすめ）", "標準入力だけ"})
		},
		"thinking": func(m *Model) { m.phase = phThinking },
		"spec": func(m *Model) {
			m.sp, m.phase = testSpec(), phSpec
			m.line.enqueue(crPart, partLabels(*m.sp)...)
		},
		"building": func(m *Model) {
			m.sp, m.phase = testSpec(), phBuilding
			m.resetGates()
			m.onEvent(factory.Event{Stage: factory.StageBuild, State: "start"})
			m.onEvent(factory.Event{Stage: factory.StageBuild, State: "ok", Source: "package main\nfunc main(){}\nfunc run(){}\n"})
			m.onEvent(factory.Event{Stage: factory.StageInspect, State: "gate", Gate: &inspect.Gate{Name: "build", OK: true}})
			m.onEvent(factory.Event{Stage: factory.StageInspect, State: "fail"})
		},
		"done": func(m *Model) {
			m.sp, m.phase = testSpec(), phDone
			m.rep = &factory.Report{Name: "demo", Lines: 120, Seconds: 12}
			m.sample = []string{"$ demo", "| a | b |", "日本語の出力"}
			m.celebrate = 60
		},
		"mix": func(m *Model) {
			m.phase, m.working = phMix, "選考係（opus）が 7 案を採点しています"
			m.line.verdicts = map[int]int{}
			m.mx = &mixState{order: MixOrder{Target: 20, PerRound: 3, Top: 3}, round: 2, built: 5, step: 3}
			m.say("pair", "数値の列をスパークラインで表示する × 干支")
			m.say("pick", "logspark  20 点  ログの行数を時間帯ごとに数えスパークラインにする")
			m.say("drop", "etospark  16 点  実用場面はほぼなく、余興の域を出ない。")
			for i, name := range []string{"logspark", "wclip", "mdgrep"} {
				m.mx.items = append(m.mx.items, mixItem{idea: mix.Idea{Name: name, Pitch: "全角を 2 桁と数え、表示幅で行を切り詰める道具"}, station: i + 1, busy: i < 2, note: "製造"})
				m.line.enqueueFor(i+1, crFunc, sMake, name)
			}
			m.mx.items[2].res = &mix.Result{Status: "failed", Reason: "inspection still failing after 2 repairs: cases"}
			m.mixLamps()
		},
		"shelf": func(m *Model) {
			m.phase = phShelf
			m.mx = &mixState{order: MixOrder{Target: 20, PerRound: 3, Top: 3}, round: 7, step: 5, total: 20, ranked: true}
			for i, name := range []string{"logspark", "wclip", "qrsplit"} {
				res := &mix.Result{Status: "ok", Report: &factory.Report{Name: name, Lines: 150}}
				m.mx.items = append(m.mx.items, mixItem{idea: mix.Idea{Name: name, Pitch: "ログの行数を時間帯ごとに数える"}, station: sShip, res: res,
					rank: i + 1, score: 90 - i*7, comment: "ログの山がどこにあるかを一行で見せるという組み合わせが意外で、すぐ試したくなる。"})
			}
		},
		"confirm": func(m *Model) {
			m.sp, m.phase = testSpec(), phConfirm
			m.rep = &factory.Report{Name: "demo"}
			m.plan.Repo, m.plan.Files = "someone/demo", []string{"main.go", "go.mod", "README.md"}
		},
	}
	sizes := [][2]int{{80, 22}, {100, 30}, {118, 40}, {161, 50}}

	for name, setup := range phases {
		for _, size := range sizes {
			m := New(Config{Build: "claude-sonnet-5-5", Desk: order.Models{Reception: "claude-haiku-5-5", Design: "claude-sonnet-5-5", Review: "claude-opus-5-5"}, Rescue: "claude-opus-5-5"})
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			setup(m)
			for i := 0; i < 40; i++ { // let the belt and the lights move
				m.frame++
				m.line.tick()
			}
			lines := strings.Split(m.render(), "\n")
			if len(lines) != size[1] {
				t.Errorf("%s at %dx%d: %d rows", name, size[0], size[1], len(lines))
			}
			for i, l := range lines {
				if w := ansi.StringWidth(l); w != size[0] {
					t.Errorf("%s at %dx%d: row %d is %d columns: %q", name, size[0], size[1], i, w, ansi.Strip(l))
					break
				}
			}
		}
	}
}

func TestSmallWindowAsksForRoom(t *testing.T) {
	m := New(Config{})
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 15})
	if got := ansi.Strip(m.render()); !strings.Contains(got, "広げてください") {
		t.Errorf("no hint for a small window: %q", got)
	}
}

// Crates must ride the belt in order and never overlap, however many are
// queued behind a closed gate.
func TestCratesQueueWithoutOverlap(t *testing.T) {
	l := newLine()
	l.layout(120)
	l.spawnAt, l.hold = sOrder, sDesign
	l.enqueue(crPart, "B1", "B2", "B3", "B4", "B5", "B6")
	for i := 0; i < 400; i++ {
		l.tick()
		for a := range l.crates {
			for b := range l.crates {
				ca, cb := l.crates[a], l.crates[b]
				if a != b && ca.x < cb.x+float64(cb.width()) && cb.x < ca.x+float64(ca.width()) {
					t.Fatalf("tick %d: %q and %q overlap", i, ca.label, cb.label)
				}
			}
		}
	}
	gate := float64(l.xs[sDesign] - machineW/2)
	for _, c := range l.crates {
		if c.x+float64(c.width()) > gate {
			t.Errorf("%q went past the closed gate", c.label)
		}
	}
	if len(l.crates) == 0 {
		t.Fatal("no crates on the belt")
	}

	// Opening the gate lets them through to the machine that takes them in.
	l.hold, l.absorb = -1, sMake
	for i := 0; i < 600; i++ {
		l.tick()
	}
	if len(l.crates) != 0 || len(l.queue) != 0 {
		t.Errorf("%d crates and %d queued were left on the belt", len(l.crates), len(l.queue))
	}
}

func TestWrapCountsWideCharacters(t *testing.T) {
	for _, s := range []string{
		"入力はどこから受けますか？引数と標準入力の両方に対応するのがおすすめです。",
		"1  Slackのエクスポート(JSONファイル)を読み、チャンネルごとのメッセージ件数を表示するプログラム",
		"plain english words that should wrap at spaces when they can",
	} {
		lines := wrap(s, 24)
		for _, l := range lines {
			if w := ansi.StringWidth(l); w > 24 {
				t.Errorf("line %q is %d columns", l, w)
			}
		}
		if got := strings.ReplaceAll(strings.Join(lines, ""), " ", ""); got != strings.ReplaceAll(s, " ", "") {
			t.Errorf("wrap lost text: %q", got)
		}
	}
	if first := wrap("1  Slackのエクスポートを読む", 20)[0]; first == "1" {
		t.Errorf("the option number was left alone on its line")
	}
}

func TestParseMix(t *testing.T) {
	for in, want := range map[string]MixOrder{
		"":           {},
		" 20":        {Target: 20},
		" QRコード":     {Seed: "QRコード"},
		" 20 QR コード": {Target: 20, Seed: "QR コード"},
		" 2000 年の暦":  {Seed: "2000 年の暦"}, // too many to be a count
	} {
		if got := parseMix(in); got != want {
			t.Errorf("parseMix(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestFuncNames(t *testing.T) {
	got := funcNames("package main\nfunc main(){}\nfunc run() int { return 0 }\nfunc aVeryLongFunctionNameIndeed(){}\n")
	want := []string{"main()", "run()", "aVeryLongFunc…()"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A proposal the desk made keeps the conversation in a state the customer
// can approve; a rejection offers numbered alternatives.
func TestDeskReplies(t *testing.T) {
	m := New(Config{Dir: t.TempDir()})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	m.onDesk(deskMsg{reply: order.Reply{Action: "reject", Message: "作れません", Options: []string{"小さい案"}}})
	if m.phase != phChat || len(m.options) != 1 {
		t.Errorf("after a rejection: phase %d, options %q", m.phase, m.options)
	}

	sp := testSpec()
	sp.Name = "kojotestnosuchcmd"
	m.onDesk(deskMsg{reply: order.Reply{Action: "propose", Message: "作ります", Spec: sp}})
	if m.phase != phSpec || m.sp == nil {
		t.Fatalf("after a proposal: phase %d", m.phase)
	}
	if len(m.line.queue) != len(sp.Behavior) {
		t.Errorf("%d parts queued for %d behaviors", len(m.line.queue), len(sp.Behavior))
	}
}
