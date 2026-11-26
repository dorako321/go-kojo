package tui

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// rgb is a 24-bit colour with bit 24 set; the zero value means "leave the
// colour that is already there".
type rgb uint32

func hex(v uint32) rgb { return rgb(v | 1<<24) }

func (c rgb) parts() (r, g, b float64) {
	return float64(c >> 16 & 0xff), float64(c >> 8 & 0xff), float64(c & 0xff)
}

func (c rgb) lip() color.Color { return lipgloss.Color(fmt.Sprintf("#%06x", uint32(c)&0xffffff)) }

// blend moves a towards b; t is clamped to [0, 1].
func blend(a, b rgb, t float64) rgb {
	t = math.Max(0, math.Min(1, t))
	ar, ag, ab := a.parts()
	br, bg, bb := b.parts()
	return hex(uint32(ar+(br-ar)*t)<<16 | uint32(ag+(bg-ag)*t)<<8 | uint32(ab+(bb-ab)*t))
}

// gradient returns the colour at position t in [0, 1] along the stops.
func gradient(t float64, stops ...rgb) rgb {
	t = math.Max(0, math.Min(1, t)) * float64(len(stops)-1)
	i := int(t)
	if i >= len(stops)-1 {
		return stops[len(stops)-1]
	}
	return blend(stops[i], stops[i+1], t-float64(i))
}

// pulse swings between 0 and 1; period is in frames.
func pulse(frame int, period float64) float64 {
	return 0.5 + 0.5*math.Sin(2*math.Pi*float64(frame)/period)
}

// noise is a repeatable pseudo-random number for a position, so the same
// star is in the same place on every frame.
func noise(x, y int) uint32 {
	h := uint32(x)*374761393 + uint32(y)*668265263
	h = (h ^ h>>13) * 1274126177
	return h ^ h>>16
}

type style struct {
	fg, bg rgb
	bold   bool
}

type cell struct {
	s    string
	st   style
	tail bool // right half of a double-width character
}

// canvas is a grid of styled cells the animated scene is painted on.
type canvas struct {
	w, h  int
	cells []cell
}

func newCanvas(w, h int, bg rgb) *canvas {
	c := &canvas{w: w, h: h, cells: make([]cell, w*h)}
	for i := range c.cells {
		c.cells[i] = cell{s: " ", st: style{bg: bg}}
	}
	return c
}

func (c *canvas) bgAt(x, y int) rgb {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return 0
	}
	return c.cells[y*c.w+x].st.bg
}

// put writes s at (x, y) and returns the column after it. Anything outside
// the canvas is clipped. A zero colour keeps what the cell already has, so
// text can be laid over a painted background.
func (c *canvas) put(x, y int, s string, st style) int {
	for _, r := range s {
		w := ansi.StringWidth(string(r))
		if w == 0 {
			continue
		}
		if y >= 0 && y < c.h && x >= 0 && x+w <= c.w {
			i := y*c.w + x
			cs := st
			if cs.bg == 0 {
				cs.bg = c.cells[i].st.bg
			}
			c.unpair(i)
			c.cells[i] = cell{s: string(r), st: cs}
			if w == 2 {
				c.unpair(i + 1)
				c.cells[i+1] = cell{tail: true, st: cs}
			}
		}
		x += w
	}
	return x
}

// unpair blanks the other half of a double-width character before one of
// its two cells is overwritten.
func (c *canvas) unpair(i int) {
	if c.cells[i].tail && i%c.w > 0 {
		c.cells[i-1].s = " "
	}
	if i%c.w < c.w-1 && c.cells[i+1].tail {
		c.cells[i+1] = cell{s: " ", st: c.cells[i+1].st}
	}
	c.cells[i].tail = false
}

// tint sets only the background of a run of cells.
func (c *canvas) tint(x, y, w int, bg rgb) {
	for i := 0; i < w; i++ {
		if x+i >= 0 && x+i < c.w && y >= 0 && y < c.h {
			c.cells[y*c.w+x+i].st.bg = bg
		}
	}
}

func (c *canvas) String() string {
	var b strings.Builder
	for y := 0; y < c.h; y++ {
		var cur style
		for x := 0; x < c.w; x++ {
			cl := c.cells[y*c.w+x]
			if cl.tail {
				continue
			}
			if x == 0 || cl.st != cur {
				cur = cl.st
				b.WriteString(sgr(cur))
			}
			b.WriteString(cl.s)
		}
		b.WriteString("\x1b[0m")
		if y < c.h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func sgr(st style) string {
	var b strings.Builder
	b.WriteString("\x1b[0")
	if st.bold {
		b.WriteString(";1")
	}
	if st.fg != 0 {
		fmt.Fprintf(&b, ";38;2;%d;%d;%d", st.fg>>16&0xff, st.fg>>8&0xff, st.fg&0xff)
	}
	if st.bg != 0 {
		fmt.Fprintf(&b, ";48;2;%d;%d;%d", st.bg>>16&0xff, st.bg>>8&0xff, st.bg&0xff)
	}
	b.WriteByte('m')
	return b.String()
}

// The night-shift palette.
var (
	cNight = hex(0x070910) // the sky and every empty cell
	cInk   = hex(0xdfe4f5)
	cDim   = hex(0x8791b0)
	cFaint = hex(0x3d4566)
	cAmber = hex(0xffb000)
	cGold  = hex(0xffe08a)
	cPink  = hex(0xff3ea5)
	cCyan  = hex(0x2de2e6)
	cGreen = hex(0x3dff9e)
	cRed   = hex(0xff4d5e)
	cSteel = hex(0x8fb4ff)
	cWall  = hex(0x12162a)
	cWall2 = hex(0x191f38)
	cBelt  = hex(0x1c2136)
	cWhite = hex(0xffffff)
)

var neon = []rgb{cPink, cAmber, cCyan, cGreen, cGold, cSteel}

func fg(c rgb) lipgloss.Style { return lipgloss.NewStyle().Foreground(c.lip()) }

// fit pads or truncates a styled line to exactly w columns.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if n := ansi.StringWidth(s); n > w {
		s = ansi.Truncate(s, w-1, "") + "…"
	}
	return s + strings.Repeat(" ", max(0, w-ansi.StringWidth(s)))
}

// box draws a rounded frame with a title around the lines; the result is
// exactly w columns by h rows.
func box(title string, lines []string, w, h int, border, titleColor rgb) []string {
	edge := fg(border)
	out := make([]string, 0, h)
	head := " " + title + " "
	out = append(out, edge.Render("╭─")+fg(titleColor).Bold(true).Render(head)+
		edge.Render(strings.Repeat("─", max(0, w-3-ansi.StringWidth(head)))+"╮"))
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, edge.Render("│")+" "+fit(line, w-4)+" "+edge.Render("│"))
	}
	out = append(out, edge.Render("╰"+strings.Repeat("─", max(0, w-2))+"╯"))
	return out
}

// wrap breaks text into lines of at most w columns, counting Japanese
// characters as two.
func wrap(s string, w int) []string {
	w = max(w, 8)
	limit := w - 2 // two columns are kept for punctuation that must not start a line
	var out []string
	for _, para := range strings.Split(s, "\n") {
		var line []rune
		width, lastSpace := 0, -1
		for _, r := range para {
			rw := ansi.StringWidth(string(r))
			if width+rw > limit && !strings.ContainsRune("、。，．）」』！？ー…", r) || width+rw > w {
				// Break at a space when one is near the end; Japanese
				// text has none, so otherwise break right here.
				cut := len(line)
				if r != ' ' && lastSpace > len(line)*3/5 {
					cut = lastSpace
				}
				rest := append([]rune(nil), line[cut:]...)
				out = append(out, strings.TrimRight(string(line[:cut]), " "))
				line = []rune(strings.TrimLeft(string(rest), " "))
				width, lastSpace = ansi.StringWidth(string(line)), -1
				if r == ' ' {
					continue
				}
			}
			if r == ' ' {
				lastSpace = len(line)
			}
			line = append(line, r)
			width += rw
		}
		out = append(out, string(line))
	}
	return out
}
