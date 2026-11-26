package tui

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/dorako321/go-kojo/internal/mix"
)

// mixItem is one row of the planning room's board: an idea on its way down
// the line or, once the jury has sat, a finished product with its score.
type mixItem struct {
	idea    mix.Idea
	station int // furthest station reached; -1 before it starts
	busy    bool
	note    string
	res     *mix.Result
	rank    int // 1, 2, 3 after the jury; 0 before
	score   int
	comment string
}

// mixState is the planning room: which round it is, how many products
// exist so far, and what is on the board.
type mixState struct {
	order  MixOrder
	step   int // 0 draw, 1 plan, 2 select, 3 build, 4 jury, 5 done
	round  int
	built  int
	total  int // products standing at the end
	ranked bool
	items  []mixItem
	ch     chan tea.Msg
	done   chan struct{} // closed when the room has stopped touching the disk
}

type (
	mixMsg    mix.Event
	mixEndMsg struct {
		out mix.Outcome
		err error
	}
)

// startMix opens the planning room: the factory invents its own orders,
// round after round, until it has made as many products as asked.
func (m *Model) startMix(o MixOrder) tea.Cmd {
	if o.Target <= 0 {
		o.Target = 3
	}
	if o.PerRound <= 0 {
		// A long run sends more ideas down the line at once, so that it
		// needs fewer rounds of planning and selection.
		o.PerRound = 3
		if o.Target >= 10 {
			o.PerRound = 5
		}
	}
	if o.Top <= 0 {
		o.Top = 3
	}
	if o.Draws <= 0 {
		o.Draws = 10
	}
	o.PerRound = min(o.PerRound, o.Target)
	m.phase, m.working = phMix, ""
	m.chat, m.turns, m.options, m.sp, m.rep = nil, nil, nil, nil, nil
	m.line = m.freshLine()
	m.line.verdicts = map[int]int{}

	what := "棚の製品や定番の道具"
	if o.Seed != "" {
		what = "「" + o.Seed + "」"
	}
	m.say("desk", fmt.Sprintf("企画会議を始めます。%sに、くじで引いた素材をかけ合わせます。%d 本できるまで続け、最後に面白かった %d 本を選びます。", what, o.Target, o.Top))

	ch, done := make(chan tea.Msg, 512), make(chan struct{})
	m.mx = &mixState{order: o, ch: ch, done: done}
	cfg := mix.LoopConfig{
		Config: mix.Config{
			Engine: m.cfg.Engine, Plan: m.cfg.Plan, Judge: m.cfg.Judge, Desk: m.cfg.Desk,
			Build: m.cfg.Build, Rescue: m.cfg.Rescue, Dir: m.cfg.Dir, Owner: m.owner(), Holder: m.cfg.Holder,
			Seed: o.Seed, Draws: o.Draws, Rand: rand.New(rand.NewSource(time.Now().UnixNano())),
		},
		Target: o.Target, PerRound: o.PerRound, Top: o.Top,
	}
	go func() {
		defer close(done)
		out, err := mix.Loop(m.ctx, cfg, func(e mix.Event) { ch <- mixMsg(e) })
		ch <- mixEndMsg{out, err}
	}()
	return wait(ch)
}

// Which station each stage of a product's journey belongs to.
var stageStation = map[string]int{
	"reception": sOrder, "design": sDesign, "review": sDesign,
	"scaffold": sMake, "build": sMake, "repair": sMake,
	"inspect": sInspect, "assemble": sShip,
}

var stageWord = map[string]string{
	"reception": "聞き取り", "design": "設計", "review": "検図",
	"scaffold": "段取り", "build": "製造", "repair": "手直し",
	"inspect": "検品", "assemble": "出荷準備",
}

func (m *Model) onMix(e mix.Event) {
	mx, l := m.mx, &m.line
	switch e.Kind {
	case "round":
		mx.round, mx.built, mx.step, mx.items = e.Round, e.Built, 0, nil
		// The belt is cleared for the new round: crates are numbered by
		// their place on the board, and the board starts again.
		l.crates, l.queue, l.verdicts = nil, nil, map[int]int{}
		if e.Round > 1 {
			m.say("desk", fmt.Sprintf("第 %d 回。ここまで %d 本。できた製品も次の元ネタに加えます。", e.Round, e.Built))
		}
	case "pairs":
		for _, p := range e.Pairs {
			seed, _, _ := strings.Cut(p.Seed, "（")
			m.say("pair", seed+" × "+p.Material)
		}
	case "planning":
		mx.step = 1
		m.working = "企画係（" + short(m.cfg.Plan) + "）が組み合わせから案を考えています"
	case "judging":
		mx.step = 2
		m.working = fmt.Sprintf("選考係（%s）が %d 案を採点しています", short(m.cfg.Judge), len(e.Ideas))
	case "picked":
		mx.step, m.working = 3, ""
		for _, i := range e.Ideas {
			if i.Picked {
				m.say("pick", fmt.Sprintf("%s  %d 点  %s", i.Name, i.Total(), i.Pitch))
				mx.items = append(mx.items, mixItem{idea: i, station: -1, note: "順番待ち"})
			} else {
				m.say("drop", fmt.Sprintf("%s  %d 点  %s", i.Name, i.Total(), i.Reason))
			}
		}
	case "ranking":
		mx.step = 4
		m.working = fmt.Sprintf("審査係（%s）が %d 本の完成品を見比べています", short(m.cfg.Judge), e.Built)
	case "product":
		if e.Product >= len(mx.items) {
			return
		}
		it, owner := &mx.items[e.Product], e.Product+1
		if e.Stage == "end" {
			it.res, it.busy = e.Result, false
			if e.Result.Status == "ok" {
				mx.built++
				m.shipped++
				it.station, it.note = sShip, "検品合格"
				m.say("note", fmt.Sprintf("%s: 検品に合格しました → ./%s/", it.idea.Name, e.Result.Report.Name))
			} else {
				it.note = "ボツ"
				l.verdicts[owner] = mkBad
				m.say("alert", fmt.Sprintf("%s: ボツ（%s）", it.idea.Name, clipWords(e.Result.Reason, 60)))
			}
			break
		}
		st, ok := stageStation[e.Stage]
		if !ok {
			return
		}
		it.station, it.busy, it.note = st, e.State == "start", stageWord[e.Stage]
		switch {
		case e.Stage == "reception" && e.State == "start":
			l.enqueue(crTicket, it.idea.Name)
		case e.Stage == "design" && e.State == "ok" && strings.HasSuffix(e.Text, "cases"):
			// The spec leaves design as a crate of parts for the builder.
			l.absorb = sMake
			l.enqueueFor(owner, crPart, sDesign, it.idea.Name)
		case e.Stage == "review" && e.State == "ok" && !strings.HasPrefix(e.Text, "0 of"):
			m.say("note", fmt.Sprintf("%s: 検図で食い違い（%s）→ 設計に戻しました", it.idea.Name, e.Text))
		case (e.Stage == "build" || e.Stage == "repair") && e.State == "start":
			l.verdicts[owner] = mkNone
			if e.Stage == "repair" {
				m.say("note", fmt.Sprintf("%s: 手直し（%s）", it.idea.Name, short(e.Text)))
			}
		case e.Source != "":
			// The written program comes out as one crate and waits at inspection.
			l.enqueueFor(owner, crFunc, sMake, it.idea.Name)
		case e.Stage == "inspect" && e.State == "ok":
			l.verdicts[owner] = mkOK
		case e.Stage == "inspect" && e.State == "fail":
			l.verdicts[owner] = mkBad
			l.halt = 6
		}
	}
	m.mixLamps()
}

// mixLamps lights each station by what the products on the line are doing.
func (m *Model) mixLamps() {
	for s := range m.line.st {
		state := stIdle
		for _, it := range m.mx.items {
			switch {
			case it.busy && it.station == s:
				state = stBusy
			case state != stBusy && it.rank == 0 && (it.station > s || (it.station == s && !it.busy)):
				state = stOK
			}
		}
		m.line.st[s] = state
	}
}

// onMixEnd puts the result on the shelf: the jury's best when there was a
// jury, otherwise whatever the last round made.
func (m *Model) onMixEnd(msg mixEndMsg) {
	mx, out := m.mx, msg.out
	m.cost += out.CostUSD
	m.working = ""
	mx.step, mx.total = 5, len(out.Built)
	m.phase = phShelf
	if len(out.Built) == 0 {
		why := "検品に合格した製品がありませんでした"
		if msg.err != nil {
			why = clipWords(msg.err.Error(), 90)
		}
		m.say("alert", "企画会議を終えました: "+why)
		return
	}
	m.celebrate = 100

	mx.ranked = len(out.Ranking) > 0 && out.Ranking[0].Score > 0
	if mx.ranked {
		mx.items = nil
		m.say("desk", fmt.Sprintf("%d 本できました（ボツ %d 本）。面白かった順に並べます。", len(out.Built), out.Dropped))
		for n, r := range out.Ranking {
			if n < mx.order.Top {
				res := r.Result
				mx.items = append(mx.items, mixItem{idea: res.Idea, station: sShip, res: &res, rank: n + 1, score: r.Score, comment: r.Comment})
				m.say("pick", fmt.Sprintf("%d 位  %s  %d 点  %s", n+1, r.Name, r.Score, r.Comment))
			} else {
				m.say("pair", fmt.Sprintf("%d 位  %s  %d 点", n+1, r.Name, r.Score))
			}
		}
	} else {
		m.say("desk", fmt.Sprintf("%d 本できました。右の棚から番号で開けます。", len(out.Built)))
	}
	if msg.err != nil {
		m.say("alert", "途中で止まりました: "+clipWords(msg.err.Error(), 80))
	}
	for s := range m.line.st {
		m.line.st[s] = stOK
	}
}

// openShelf shows one finished product on the shipping label, from where it
// can be installed or published like any other.
func (m *Model) openShelf(n int) {
	if n >= len(m.mx.items) {
		return
	}
	res := m.mx.items[n].res
	if res == nil || res.Status != "ok" {
		return
	}
	m.sp, m.rep = res.Spec, res.Report
	m.sample = m.sampleRun(*res.Report)
	m.installed, m.repoURL = "", ""
	m.phase = phDone
}

func clipWords(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// mixBoard is the right-hand pane while the planning room is open.
func (m *Model) mixBoard(w int, key func(k, text string) string) []string {
	mx := m.mx
	steps := []string{"くじ引き", "企画", "選考", "製造", "審査", "結果"}
	var bar []string
	for i, s := range steps {
		switch {
		case i < mx.step:
			bar = append(bar, fg(dimmed(cGreen)).Render(s))
		case i == mx.step:
			bar = append(bar, fg(cGold).Bold(true).Render(s))
		default:
			bar = append(bar, fg(cFaint).Render(s))
		}
	}
	progress := fmt.Sprintf("第 %d 回 ・ 完成 %d / %d 本", max(mx.round, 1), mx.built, mx.order.Target)
	if mx.step == 5 {
		progress = fmt.Sprintf("全 %d 回 ・ 完成 %d 本", max(mx.round, 1), mx.total)
	}
	lines := []string{strings.Join(bar, fg(cFaint).Render(" → ")), fg(cDim).Render(progress), ""}

	switch {
	case mx.ranked:
		lines = append(lines, fg(cGold).Bold(true).Render(fmt.Sprintf("面白かった %d 選", len(mx.items))), "")
	case len(mx.items) == 0 && mx.step < 5:
		lines = append(lines,
			fg(cDim).Render("元ネタと無関係な素材を、くじで組にします。"),
			fg(cDim).Render("企画係が組ごとに案を書き、選考係が点をつけます。"),
			fg(cDim).Render("作るのは上位だけ。あとはボツです。"))
	}
	for n, it := range mx.items {
		head := fg(cNight).Background(cAmber.lip()).Bold(true).Render(fmt.Sprintf(" %d ", n+1)) + " " + fg(cGold).Bold(true).Render(it.idea.Name)
		if it.rank > 0 {
			head += "  " + fg(cPink).Bold(true).Render(fmt.Sprintf("%d 点", it.score))
		}
		lines = append(lines, head+"  "+fg(cInk).Render(it.idea.Pitch))
		if it.rank > 0 {
			// After the jury: the score and why, wrapped to the pane.

			// Two lines of the jury's comment; the whole of it is in the
			// conversation on the left.
			comment := wrap(it.comment, w-4)
			if len(comment) > 2 {
				comment = append(comment[:1], strings.TrimRight(comment[1], "、。 ")+"…")
			}
			for _, l := range comment {
				lines = append(lines, "    "+fg(cInk).Render(l))
			}
		} else {
			var lamps []string
			for s, name := range stationNames {
				col, mark := cFaint, "○"
				switch {
				case it.res != nil && it.res.Status != "ok" && s == max(it.station, 0):
					col, mark = cRed, "✕"
				case it.busy && it.station == s:
					col, mark = blend(cAmber, cWhite, pulse(m.frame, 8)), "◉"
				case it.station > s || (it.station == s && !it.busy):
					col, mark = cGreen, "●"
				}
				lamps = append(lamps, fg(col).Render(mark+" "+name))
			}
			status := fg(cDim).Render(it.note)
			if it.res != nil && it.res.Status != "ok" {
				status = fg(cRed).Render("ボツ")
			}
			lines = append(lines, "    "+strings.Join(lamps, " ")+"  "+status)
		}
		switch {
		case it.res != nil && it.res.Status == "ok":
			r := it.res.Report
			lines = append(lines, "    "+fg(cGreen).Render(fmt.Sprintf("./%s/%s -h", r.Name, r.Name))+
				fg(cDim).Render(fmt.Sprintf("   %d 行・手直し %d 回", r.Lines, r.Repairs())))
		case it.res != nil:
			lines = append(lines, "    "+fg(cRed).Render(clipWords(it.res.Reason, 46)))
		}
		lines = append(lines, "")
	}
	if m.phase == phShelf {
		open := ""
		if len(mx.items) > 0 {
			open = key(fmt.Sprintf("1-%d", len(mx.items)), "開く") + "  "
		}
		lines = append(lines, open+key("m", "もう一回"), key("n", "注文に戻る")+"  "+key("q", "終了"))
	}
	return lines
}

// dimmed tones a colour down for steps that are behind us.
func dimmed(c rgb) rgb { return blend(c, cNight, 0.35) }
