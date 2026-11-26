package tui

import (
	"sort"

	"github.com/charmbracelet/x/ansi"
)

// The five stations of the line, left to right.
const (
	sOrder = iota
	sDesign
	sMake
	sInspect
	sShip
)

var stationNames = [5]string{"受注", "設計", "製造", "検品", "出荷"}

// What a station's lamp shows.
const (
	stIdle = iota
	stBusy
	stOK
	stBad
)

// Kinds of crate on the belt.
const (
	crTicket = iota // the customer's order on its way to the desk
	crPart          // one numbered behaviour from the spec
	crFunc          // one function of the written program
)

// Marks stamped on a crate at inspection.
const (
	mkNone = iota
	mkOK
	mkBad
)

const (
	lineRows   = 6
	beltSpeed  = 0.55
	machineW   = 11
	crateSpace = 1
)

type crate struct {
	label string
	x     float64 // left edge, in cells
	kind  int
	mark  int
	fade  int // frames a rejected crate stays visible

	// When several products are on the line at once, a crate belongs to one
	// of them (1, 2, ...) and starts after a station of its own; inspection
	// then holds and stamps it by that product's verdict, not the line's.
	owner int
	from  int
}

func (c crate) width() int { return ansi.StringWidth(c.label) + 2 }

// line is the conveyor: crates ride it from station to station, and what
// happens to them at each station is set by the phase of the build.
type line struct {
	w        int
	xs       [5]int // centre column of each station
	st       [5]int
	crates   []crate
	queue    []crate  // crates waiting to be put on the belt
	spawnAt  int      // station they appear after
	loop     []string // part labels fed again and again while the engine works
	loopI    int
	cool     int
	hold     int // crates queue up in front of this station; -1 for none
	absorb   int // parts disappear into this station; -1 for none
	verdict  int // mark given to function crates passing inspection
	halt     int // frames the belt stands still after a rejection
	travel   float64
	workers  [5]string   // who staffs each station: a model, or plain code
	verdicts map[int]int // inspection verdict per owner, for crates that have one
}

func newLine() line { return line{hold: -1, absorb: -1} }

func (l *line) layout(w int) {
	l.w = w
	margin := machineW/2 + 3
	for i := range l.xs {
		l.xs[i] = margin + i*(w-2*margin)/4
	}
}

func (l *line) enqueue(kind int, labels ...string) {
	for _, s := range labels {
		l.queue = append(l.queue, crate{label: s, kind: kind})
	}
}

// enqueueFor puts one crate of a given product on the belt after a station.
func (l *line) enqueueFor(owner, kind, from int, label string) {
	l.queue = append(l.queue, crate{label: label, kind: kind, owner: owner, from: from})
}

// verdictFor is the stamp a crate gets at inspection: its product's when it
// has an owner, otherwise the line's.
func (l *line) verdictFor(c crate) int {
	if c.owner != 0 {
		return l.verdicts[c.owner]
	}
	return l.verdict
}

func (l *line) tick() {
	if l.halt > 0 {
		l.halt--
		return
	}
	l.travel += beltSpeed

	// Put the next crate on the belt when there is room behind the station.
	if l.cool > 0 {
		l.cool--
	}
	if len(l.queue) == 0 && len(l.loop) > 0 {
		l.enqueue(crPart, l.loop[l.loopI%len(l.loop)])
		l.loopI++
	}
	if l.cool == 0 && len(l.queue) > 0 {
		c := l.queue[0]
		at := l.spawnAt
		if c.owner != 0 {
			at = c.from
		}
		c.x = float64(l.xs[at] + machineW/2 + 1)
		if c.kind == crTicket {
			c.x = 0
		}
		free := true
		for _, o := range l.crates {
			if o.x < c.x+float64(c.width()+crateSpace) && o.x+float64(o.width()) > c.x-1 {
				free = false
			}
		}
		if free {
			l.crates = append(l.crates, c)
			l.queue = l.queue[1:]
			l.cool = 7
		}
	}

	// Move, front crate first, so nothing drives into the one ahead.
	sort.SliceStable(l.crates, func(i, j int) bool { return l.crates[i].x > l.crates[j].x })
	ahead := float64(l.w + 100)
	for i := range l.crates {
		c := &l.crates[i]
		next := c.x + beltSpeed
		hold := l.hold
		if c.owner != 0 {
			// A product's crate waits at inspection until its own verdict is in.
			hold = -1
			if c.kind == crFunc && c.mark == mkNone && l.verdicts[c.owner] == mkNone {
				hold = sInspect
			}
		}
		if hold >= 0 && c.kind != crTicket {
			if gate := float64(l.xs[hold] - machineW/2 - 1 - c.width()); c.x <= gate && next > gate {
				next = gate
			}
		}
		if limit := ahead - float64(c.width()+crateSpace); next > limit {
			next = max(c.x, limit)
		}
		c.x = next
		ahead = c.x
	}

	// What the stations do to the crates passing through them.
	kept := l.crates[:0]
	for _, c := range l.crates {
		mid := int(c.x) + c.width()/2
		switch {
		case c.kind == crTicket && mid >= l.xs[sOrder]:
			continue
		case c.kind == crPart && l.absorb >= 0 && mid >= l.xs[l.absorb]:
			continue
		case c.fade > 0:
			if c.fade--; c.fade == 0 {
				continue
			}
		case c.kind == crFunc && c.mark == mkNone && mid >= l.xs[sInspect] && l.verdictFor(c) != mkNone:
			if c.mark = l.verdictFor(c); c.mark == mkBad {
				c.fade = 14
			}
		case c.kind == crFunc && mid >= l.xs[sShip]:
			continue
		case int(c.x) >= l.w:
			continue
		}
		kept = append(kept, c)
	}
	l.crates = kept
}

// reject stops the belt for a moment, the way a line stops on a defect.
func (l *line) reject() {
	l.verdict, l.hold, l.halt = mkBad, -1, 10
}

func (l *line) draw(c *canvas, y, frame int) {
	// Belt and rollers.
	for x := 0; x < c.w; x++ {
		phase := ((x-int(l.travel))%6 + 6) % 6
		glyph, col := "━", blend(cFaint, cNight, 0.25)
		switch phase {
		case 0:
			glyph, col = "╋", cDim
		case 5:
			col = cFaint
		}
		c.put(x, y+4, glyph, style{fg: col, bg: cBelt})
		// Rollers under the belt, and between them a row of running
		// lights that chase in the direction the belt moves.
		switch {
		case x%6 == 3:
			c.put(x, y+5, []string{"◐", "◓", "◑", "◒"}[int(l.travel/1.6)%4], style{fg: cDim})
		case x%2 == 0:
			led := gradient(float64(x)/float64(max(1, c.w)), cPink, cAmber, cCyan)
			if ((x/2-int(l.travel*0.9))%5+5)%5 != 0 || l.halt > 0 {
				led = blend(led, cNight, 0.78)
			}
			c.put(x, y+5, "•", style{fg: led, bold: true})
		}
	}

	// Crates ride on the belt, behind the machine legs.
	for _, cr := range l.crates {
		st := style{fg: cSteel, bg: hex(0x1b2a55)}
		label := cr.label
		switch {
		case cr.kind == crTicket:
			st = style{fg: cCyan, bg: hex(0x0d3a40), bold: true}
		case cr.mark == mkOK:
			st = style{fg: cGreen, bg: hex(0x0c4029), bold: true}
		case cr.mark == mkBad:
			st = style{fg: cWhite, bg: blend(cRed, hex(0x4a1018), pulse(frame, 6)), bold: true}
		case cr.kind == crFunc:
			st = style{fg: cGold, bg: hex(0x4a3200), bold: true}
		}
		c.put(int(cr.x), y+3, " "+label+" ", st)
		// A lid above the label makes the crate read as a box. The
		// machines are drawn afterwards, so a crate passes under them.
		for i := 0; i < cr.width(); i++ {
			c.put(int(cr.x)+i, y+2, "▄", style{fg: blend(st.bg, st.fg, 0.35)})
		}
	}

	// Machines stand over the belt.
	for i, name := range stationNames {
		x := l.xs[i] - machineW/2
		edge := hex(0x4d5780)
		lamp := cFaint
		meter := "▁▁▁▁▁"
		meterCol := cFaint
		switch l.st[i] {
		case stBusy:
			edge = blend(cAmber, cGold, pulse(frame, 14))
			lamp = blend(cAmber, cWhite, pulse(frame, 8))
		case stOK:
			edge, lamp, meter, meterCol = blend(cGreen, cNight, 0.45), cGreen, "▇▇▇▇▇", blend(cGreen, cNight, 0.3)
		case stBad:
			edge = blend(cRed, hex(0x4a1018), pulse(frame, 8))
			lamp, meter, meterCol = edge, "✕ NG ", cRed
		}
		es := style{fg: edge, bg: cWall, bold: l.st[i] == stBusy}
		c.put(x, y+1, "┏━━ ", es)
		c.put(x+4, y+1, name, style{fg: blend(cInk, edge, 0.25), bg: cWall, bold: true})
		c.put(x+8, y+1, " ━┓", es)
		c.put(x, y+2, "┃         ┃", es)
		c.put(x+2, y+2, "●", style{fg: lamp, bg: cWall})
		if l.st[i] == stBusy {
			for k := 0; k < 5; k++ {
				lv := pulse(frame+k*4, 13)
				c.put(x+4+k, y+2, string([]rune("▁▂▃▄▅▆▇█")[int(lv*7.99)]),
					style{fg: gradient(lv, cAmber, cPink), bg: cWall})
			}
			// Sparks fly while a machine works.
			for k := 0; k < 4; k++ {
				n := noise(frame/2+k*31, l.xs[i])
				if n%3 != 0 {
					continue
				}
				c.put(x+int(n>>3%machineW), y, []string{"✦", "*", ".", "+"}[n>>9%4],
					style{fg: []rgb{cGold, cWhite, cPink, cAmber}[n>>13%4], bold: true})
			}
		} else {
			c.put(x+4, y+2, meter, style{fg: meterCol, bg: cWall})
		}
		c.put(x, y+3, "┃", style{fg: edge})
		c.put(x+machineW-1, y+3, "┃", style{fg: edge})

		// Over each machine, who works there.
		tag, tagCol := l.workers[i], cFaint
		if l.st[i] == stBusy {
			tagCol = cGold
		} else if l.st[i] == stOK {
			tagCol = blend(cGreen, cNight, 0.35)
		}
		c.put(l.xs[i]-ansi.StringWidth(tag)/2, y, tag, style{fg: tagCol, bold: l.st[i] == stBusy})
	}

}
