package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

const (
	minW = 80
	minH = 22
)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.BackgroundColor = cNight.lip()
	v.WindowTitle = "KOJO"
	// The input is the last row inside the left pane: above it are only the
	// pane's bottom border and the key hints, left of it the border and a space.
	if m.typing() && m.w >= minW && m.h >= minH {
		if c := m.input.Cursor(); c != nil {
			c.Position.X += 2
			c.Position.Y += m.h - 3
			v.Cursor = c
		}
	}
	return v
}

// render lays out the whole screen: night sky, conveyor, the two panes
// and the key hints.
func (m *Model) render() string {
	if m.w == 0 {
		return ""
	}
	if m.w < minW || m.h < minH {
		return night(fit(fmt.Sprintf(" KOJO: 画面を %d×%d 以上に広げてください（いま %d×%d）", minW, minH, m.w, m.h), m.w), m.w)
	}

	head := sceneRows(m.h)
	c := newCanvas(m.w, head+lineRows, cNight)
	drawScene(c, head, m.frame, mood{
		busy:      m.phase == phBuilding || m.phase == phPublishing || m.phase == phMix,
		celebrate: m.celebrate,
		shipped:   m.shipped,
		status:    fmt.Sprintf("本日の出荷 %d 台 ・ 利用額の目安 $%.2f", m.shipped, m.cost),
	})
	m.line.draw(c, head, m.frame)

	paneH := m.h - head - lineRows - 1
	leftW := m.w * 55 / 100
	left := box("受注窓口", m.chatLines(leftW-4, paneH-2), leftW, paneH, hex(0x39507a), cCyan)
	title, lines, edge := m.board(m.w-leftW-4, paneH-2)
	right := box(title, lines, m.w-leftW, paneH, edge, cAmber)

	var b strings.Builder
	b.WriteString(c.String())
	for i := range left {
		b.WriteString("\n" + left[i] + right[i])
	}
	b.WriteString("\n" + fit(m.footer(), m.w))
	return night(b.String(), m.w)
}

// night gives every cell the night background and the pale ink colour, so
// the screen looks the same whatever theme the terminal has.
func night(s string, w int) string {
	base := sgr(style{fg: cInk, bg: cNight})
	s = strings.NewReplacer("\x1b[0m", base, "\x1b[m", base).Replace(s)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = base + l + strings.Repeat(" ", max(0, w-ansi.StringWidth(l))) + "\x1b[0m"
	}
	return strings.Join(lines, "\n")
}

// chatLines renders the conversation and keeps the newest part in view.
func (m *Model) chatLines(w, h int) []string {
	var out []string
	add := func(mark string, markCol, textCol rgb, bold bool, text string) {
		for i, l := range wrap(text, w-2) {
			lead := "  "
			if i == 0 {
				lead = fg(markCol).Bold(true).Render(mark) + " "
			}
			out = append(out, lead+fg(textCol).Bold(bold).Render(l))
		}
	}
	for i, l := range m.chat {
		if i > 0 && l.role != "option" && m.chat[i-1].role != l.role {
			out = append(out, "")
		}
		switch l.role {
		case "customer":
			add("▸", cCyan, cCyan, true, l.text)
		case "desk":
			add("◂", cAmber, cInk, false, l.text)
		case "option":
			add(" ", cPink, cGold, false, l.text)
		case "alert":
			add("◆", cRed, cRed, true, l.text)
		case "pair":
			add("・", cFaint, cDim, false, l.text)
		case "pick":
			add("★", cGold, cGold, true, l.text)
		case "drop":
			add("－", cFaint, blend(cDim, cNight, 0.3), false, "ボツ  "+l.text)
		default:
			add("◆", cDim, cDim, false, l.text)
		}
	}
	if m.phase == phThinking || (m.phase == phMix && m.working != "") {
		doing := m.working
		if doing == "" {
			doing = "受付に渡しています"
		}
		out = append(out, "", fg(cAmber).Render("◂ "+spinner[m.frame/2%len(spinner)]+" "+doing+"…"))
	}

	// The last two rows belong to the input, or to a hint when keys are
	// taken directly.
	var tail []string
	rule := fg(hex(0x26304f)).Render(strings.Repeat("─", w))
	switch {
	case m.typing():
		tail = []string{rule, m.input.View()}
	case m.phase == phThinking || m.phase == phBuilding || m.phase == phPublishing:
		tail = []string{rule, fg(cFaint).Render("（作業中です。しばらくお待ちください）")}
	case m.phase == phMix:
		tail = []string{rule, fg(cFaint).Render("（企画会議中です。工場におまかせください）")}
	case m.phase == phShelf:
		tail = []string{rule, fg(cFaint).Render("（右の棚のキーで操作します）")}
	default:
		tail = []string{rule, fg(cFaint).Render("（右のラベルのキーで操作します）")}
	}
	room := h - len(tail)
	if len(out) > room {
		out = out[len(out)-room:]
	}
	for len(out) < room {
		out = append(out, "")
	}
	return append(out, tail...)
}

// board is the right-hand pane; what it shows follows the phase.
func (m *Model) board(w, h int) (title string, lines []string, edge rgb) {
	key := func(k, text string) string {
		return fg(cNight).Background(cAmber.lip()).Bold(true).Render(" "+k+" ") + " " + fg(cInk).Render(text)
	}
	label := func(k, v string) string { return fg(cDim).Render(fit(k, 8)) + fg(cInk).Render(v) }
	edge = hex(0x5a4a26)

	switch m.phase {
	case phMix, phShelf:
		title, edge = "企画会議", blend(cPink, cNight, 0.45)
		if m.phase == phShelf {
			title, edge = "棚", blend(cGreen, cNight, 0.5)
		}
		lines = m.mixBoard(w, key)
	case phSpec:
		title = "指示書"
		lines = m.specCard(w, label)
		lines = append(lines, "", key("⏎", "この内容で製造開始")+"   "+fg(cDim).Render("直したい点は入力して送信"))
	case phBuilding:
		title, edge = "検品ボード", blend(cAmber, hex(0x5a4a26), pulse(m.frame, 20))
		lines = append(lines,
			fg(cGold).Bold(true).Render(m.sp.Name)+fg(cDim).Render(fmt.Sprintf("   試行 %d/3 ・ %ds", max(1, m.attempt), int(time.Since(m.started).Seconds()))), "")
		if m.line.st[sMake] == stBusy {
			lines = append(lines, fg(cAmber).Render(spinner[m.frame/2%len(spinner)]+" "+short(m.maker)+" が main.go を書いています…"), "")
		} else {
			lines = append(lines, fg(cGreen).Render("✓ main.go が書き上がりました"), "")
		}
		for _, g := range m.gates {
			mark, col := "●", cFaint
			switch g.state {
			case stOK:
				mark, col = "●", cGreen
			case stBad:
				mark, col = "✕", cRed
			}
			lines = append(lines, fg(col).Bold(true).Render(mark)+" "+fg(blend(cInk, col, 0.3)).Render(fit(g.name, 9))+fg(cDim).Render(gateHelp[g.name]))
		}
	case phDone, phPublished:
		title, edge = "出荷ラベル", blend(cGreen, cNight, 0.5)
		r := m.rep
		lines = append(lines,
			fg(cGreen).Bold(true).Render("▣ "+r.Name)+"  "+fg(cInk).Render(m.sp.Summary), "",
			label("場所", "./"+r.Name+"/"),
			label("試す", "./"+r.Name+"/"+r.Name+" -h"),
			label("中身", fmt.Sprintf("main.go %d 行 ・ 手直し %d 回 ・ %.0f 秒", r.Lines, r.Repairs(), r.Seconds)))
		if m.installed != "" {
			lines = append(lines, label("PATH", m.installed))
		}
		if m.repoURL != "" {
			lines = append(lines, label("公開", m.repoURL),
				label("導入", "go install "+strings.TrimPrefix(m.repoURL, "https://")+"@latest"))
		}
		lines = append(lines, "")
		for i, s := range m.sample {
			if i == 7 {
				lines = append(lines, fg(cFaint).Render("…"))
				break
			}
			col := cSteel
			if i == 0 {
				col = cDim
			}
			lines = append(lines, fg(col).Render(s))
		}
		lines = append(lines, "")
		if m.phase == phDone {
			lines = append(lines, key("p", "publish（GitHub に公開）")+"  "+key("i", "PATH に置く"))
		}
		back := ""
		if m.mx != nil {
			back = key("b", "棚に戻る") + "  "
		}
		lines = append(lines, back+key("n", "次の注文")+"  "+key("q", "終了"))
	case phConfirm, phPublishing:
		title, edge = "公開の確認", blend(cPink, cNight, 0.3)
		lines = append(lines,
			fg(cPink).Bold(true).Render("GitHub に public で公開します"), "",
			label("宛先", "github.com/"+m.plan.Repo),
			label("中身", strings.Join(m.plan.Files, "  ")), "",
			fg(cDim).Render("kojo.json には依頼文と仕様が入っています。"),
			fg(cDim).Render("公開後は誰でも go install で導入できます。"), "")
		if m.phase == phPublishing {
			lines = append(lines, fg(cAmber).Render(spinner[m.frame/2%len(spinner)]+" 公開しています…"))
		} else {
			lines = append(lines, key("y", "公開する")+"  "+key("n", "戻る"))
		}
	default:
		title = "作れるもの"
		for _, row := range [][2]string{
			{"変換", "CSV を Markdown の表に / 全角半角をそろえる"},
			{"検査", ".env のキー漏れ / 証明書の残り日数"},
			{"表示", "QR コード / 頻度の棒グラフ"},
			{"生成", "合言葉 / ダミーデータ"},
			{"取得", "公開 API を 1 行で（通信の許可が要ります）"},
		} {
			lines = append(lines, fg(cGold).Bold(true).Render(fit(row[0], 6))+fg(cInk).Render(row[1]))
		}
		lines = append(lines, "",
			label("規格", "main.go 1 枚・200 行まで・フラグ 3 つまで"),
			label("不可", "全画面アプリ / ログインが要るもの / 常駐もの"), "",
			fg(cDim).Render("作れない注文は、理由と代わりの案をお伝えします。"), "",
			fg(cAmber).Bold(true).Render("担当"))
		staff := [][3]string{
			{"受付", short(m.cfg.Desk.Reception), "聞き取り。作れない注文は断る"},
			{"設計", short(m.cfg.Desk.Design), "仕様と受け入れケースを書く"},
			{"検図", short(m.cfg.Desk.Review), "期待値を別モデルで検算する"},
			{"製造", short(m.cfg.Build), "main.go を書く"},
			{"救援", short(m.cfg.Rescue), "最後の手直しだけ引き受ける"},
			{"検品", "go", "合否はコードが判定する"},
			{"企画", short(m.cfg.Plan), "/mix で、組み合わせから案を出す"},
			{"選考", short(m.cfg.Judge), "/mix で、案に点をつける"},
		}
		for _, r := range staff {
			if r[1] == "" {
				continue
			}
			lines = append(lines, fg(cGold).Bold(true).Render(fit(r[0], 6))+fg(cCyan).Render(fit(r[1], 8))+fg(cDim).Render(r[2]))
		}
		if m.sp != nil && m.rep == nil {
			lines = append(lines, "", fg(cDim).Render("直前の指示書: "+m.sp.Name))
		}
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "…")
	}
	if len(lines) > h {
		// Keep the keys at the bottom visible when the pane is short.
		lines = append(lines[:h-2], lines[len(lines)-2:]...)
	}
	return title, lines, edge
}

func (m *Model) specCard(w int, label func(k, v string) string) []string {
	s := m.sp
	lines := []string{
		fg(cGold).Bold(true).Render(s.Name) + "  " + fg(cPink).Render(s.Kind),
		fg(cInk).Render(s.Summary), "",
		label("使い方", s.Usage),
	}
	for _, f := range s.Flags {
		lines = append(lines, label("フラグ", "-"+f.Name+"  "+f.Help))
	}
	deps := "標準ライブラリのみ"
	if len(s.Deps) > 0 {
		deps = strings.Join(s.Deps, ", ")
	}
	net := "しない"
	if s.Network {
		net = "する"
	}
	lines = append(lines, label("依存", deps), label("通信", net),
		label("検品", fmt.Sprintf("%d ケース（実機で実行）", len(s.Cases))), "")
	for _, b := range s.Behavior {
		lines = append(lines, fg(cDim).Render(b))
	}
	return lines
}

func (m *Model) footer() string {
	hint := func(k, text string) string { return fg(cAmber).Bold(true).Render(k) + " " + fg(cDim).Render(text) }
	var parts []string
	switch m.phase {
	case phMix:
		parts = []string{fg(cDim).Render("企画会議中…")}
	case phShelf:
		parts = []string{hint("1-9", "開く"), hint("m", "もう一回"), hint("n", "注文に戻る"), hint("q", "終了")}
	case phChat:
		parts = []string{hint("⏎", "送信"), hint("/mix", "おまかせで作る（/mix 20 で 20 本）")}
		if len(m.options) > 0 {
			parts = append(parts, hint(fmt.Sprintf("1-%d", len(m.options)), "番号で選ぶ"))
		}
	case phSpec:
		parts = []string{hint("⏎", "製造開始"), hint("入力+⏎", "仕様を直す")}
	case phDone:
		parts = []string{hint("p", "publish"), hint("i", "PATH に置く"), hint("n", "次の注文"), hint("q", "終了")}
	case phPublished:
		parts = []string{hint("n", "次の注文"), hint("q", "終了")}
	case phConfirm:
		parts = []string{hint("y", "公開する"), hint("n", "戻る")}
	default:
		parts = []string{fg(cDim).Render("作業中…")}
	}
	parts = append(parts, hint("ctrl+c", "中止して終了"))
	return " " + strings.Join(parts, fg(cFaint).Render("  ・  "))
}

// Run opens the factory floor and blocks until the user leaves.
func Run(cfg Config) error {
	m := New(cfg)
	defer m.cancel()
	var opts []tea.ProgramOption
	// Terminals inside tmux are often detected as 256-colour even when they
	// can show more; KOJO_COLOR=truecolor overrides the detection.
	if os.Getenv("KOJO_COLOR") == "truecolor" {
		opts = append(opts, tea.WithColorProfile(colorprofile.TrueColor))
	}
	_, err := tea.NewProgram(m, opts...).Run()

	// Leaving in the middle of a build must not leave half a product
	// behind: wait for the line to stop, then clear what it had made.
	m.cancel()
	if m.mx != nil {
		select {
		case <-m.mx.done:
		case <-time.After(15 * time.Second):
		}
	}
	if m.inFlight != nil {
		select {
		case rep := <-m.inFlight:
			if rep.Created && rep.Status != "ok" {
				os.RemoveAll(rep.Dir)
			}
		case <-time.After(10 * time.Second):
		}
	}
	return err
}
