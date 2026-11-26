package tui

import (
	"math"
)

// logo is the rooftop neon sign.
var logo = []string{
	"█ ▄▀ ▄▀▀▄    █ ▄▀▀▄",
	"██   █  █    █ █  █",
	"█ ▀▄ ▀▄▄▀ ▀▄▄▀ ▀▄▄▀",
}

const logoW = 19

// sceneRows is how tall the night-sky header is for a given window height.
func sceneRows(h int) int {
	switch {
	case h >= 36:
		return 10
	case h >= 27:
		return 6
	}
	return 1
}

type building struct {
	x, w, h int
	kind    int // 0 flat, 1 sawtooth, 2 tower, 3 chimney, 4 fence
}

// skyline lays buildings across the width. It depends only on the width, so
// the town does not rearrange itself between frames.
func skyline(w, rows, signX, signW, clearTo int) []building {
	var out []building
	for x, i := 0, 0; x < w; i++ {
		n := noise(i, 11)
		b := building{x: x}
		switch {
		case signW > 0 && x >= signX-1 && x < signX+signW:
			// The sign needs a flat roof to stand on.
			b.w, b.h = signX+signW+1-x, rows
		case n%8 < 2:
			b.w, b.h = 6+int(n>>4%6), max(1, rows-1-int(n>>8%2))
		case n%8 < 4:
			b.w, b.h, b.kind = 10+int(n>>4%8), max(1, rows-2), 1
		case n%8 == 4:
			b.w, b.h, b.kind = 4+int(n>>4%2), rows, 2
		case n%8 == 5 && x > clearTo && (len(out) == 0 || out[len(out)-1].kind != 3):
			// Chimneys stand clear of the sign and the words beside it.
			b.w, b.h, b.kind = 2, rows+2, 3
		default:
			b.w, b.h, b.kind = 1+int(n>>4%2), 1, 4
		}
		// Do not start a building that would run into the sign.
		if signW > 0 && b.x < signX-1 && b.x+b.w > signX-1 {
			b.w = signX - 1 - b.x
		}
		out = append(out, b)
		x += b.w
	}
	return out
}

// mood is what the rest of the screen tells the scene.
type mood struct {
	busy      bool // the line is working: chimneys smoke harder
	celebrate int  // frames of fireworks left
	shipped   int  // products shipped this session
	cost      float64
	status    string // short line under the tagline
}

// drawScene paints the sky, the town and the sign into the top of c.
func drawScene(c *canvas, rows, frame int, md mood) {
	if rows <= 1 {
		x := c.put(1, 0, "KOJO", style{fg: gradient(pulse(frame, 60), cPink, cAmber, cCyan), bold: true})
		x = c.put(x+1, 0, "夜間操業中", style{fg: cGold})
		c.put(x+2, 0, md.status, style{fg: cDim})
		return
	}
	cityRows := 4
	framed := rows >= 10
	if !framed {
		cityRows = 3
	}
	skyRows := rows - cityRows

	// Stars. Each has its own rhythm.
	for y := 0; y < skyRows; y++ {
		for x := 0; x < c.w; x++ {
			n := noise(x, y*7+3)
			if n%41 != 0 {
				continue
			}
			p := pulse(frame+int(n>>8%97), 40+float64(n>>16%50))
			glyph, col := ".", blend(cFaint, cSteel, p)
			if p > 0.93 {
				glyph, col = "✦", cWhite
			} else if p > 0.75 {
				glyph, col = "*", blend(cSteel, cWhite, p)
			}
			c.put(x, y, glyph, style{fg: col})
		}
	}

	// Fireworks when a product ships.
	if md.celebrate > 0 {
		for b := 0; b < 4; b++ {
			age := float64(100-md.celebrate) - float64(b)*14
			if age < 0 || age > 34 {
				continue
			}
			cx := c.w*(2+b*2)/10 + int(noise(b, md.shipped)%7)
			cy := 1 + int(noise(b+9, md.shipped)%uint32(max(1, skyRows-2)))
			col := blend(neon[(b+md.shipped)%len(neon)], cNight, age/45)
			for k := 0; k < 14; k++ {
				a := 2 * math.Pi * float64(k) / 14
				x := cx + int(math.Round(math.Cos(a)*age*0.75))
				y := cy + int(math.Round(math.Sin(a)*age*0.28))
				if y >= skyRows {
					continue
				}
				glyph := "✦"
				if age > 22 {
					glyph = "."
				} else if age > 10 {
					glyph = "*"
				}
				c.put(x, y, glyph, style{fg: col, bold: true})
			}
		}
	}

	// Where the sign and the words beside it go.
	signX, signY, signW := 3, 0, logoW
	if framed {
		signY, signW = 1, logoW+6
	}
	tx := signX + signW + 3
	ty := signY
	if framed {
		ty = signY + 1
	}

	// The town.
	base := rows - 1
	city := skyline(c.w, cityRows, signX, ifInt(framed, signW, 0), tx+48)
	for i, b := range city {
		wall := cWall
		if i%2 == 1 {
			wall = cWall2
		}
		top := base - b.h + 1
		for y := top; y <= base; y++ {
			for x := b.x; x < b.x+b.w; x++ {
				c.put(x, y, " ", style{bg: wall})
			}
		}
		switch b.kind {
		case 3: // chimney: warning light on top, smoke above
			c.put(b.x, top, "▀▀", style{fg: blend(cRed, hex(0x3a0e16), pulse(frame+i*9, 20)), bg: wall})
			puffs := 2
			if md.busy {
				puffs = 5
			}
			for k := 1; k <= puffs; k++ {
				if (k+frame/5+i)%3 == 0 {
					continue
				}
				drift := (k*k + frame/7) / 3 % 4
				glyph := "░"
				if k <= 2 && md.busy {
					glyph = "▒"
				}
				c.put(b.x+drift, top-k, glyph, style{fg: blend(hex(0x59628a), cNight, float64(k)/6.5)})
			}
		case 4:
			c.put(b.x, top, "▂", style{fg: cFaint, bg: cNight})
		default:
			// Windows: mostly steady, a few switching on and off.
			for y := top + 1; y <= base; y++ {
				for x := b.x + 1; x < b.x+b.w-1; x += 2 {
					n := noise(x, y*13+i)
					lit := (n+uint32(frame/50))%7 < 4
					col := hex(0x262d4a)
					if lit {
						col = blend(cAmber, cGold, pulse(frame+int(n%60), 35+float64(n>>9%40)))
						if n%13 == 0 {
							col = blend(cCyan, cSteel, pulse(frame+int(n%31), 28))
						}
					}
					c.put(x, y, "▪", style{fg: col})
				}
			}
			// A string of lights along the roof, chasing to the right.
			for x := b.x; x < b.x+b.w; x += 2 {
				col := neon[(x/2+i)%len(neon)]
				if (x/2-frame/3)%4 != 0 {
					col = blend(col, wall, 0.62)
				}
				c.put(x, top, "•", style{fg: col, bold: true})
			}
			if b.kind == 1 { // sawtooth roof
				for x := b.x; x+1 < b.x+b.w; x += 2 {
					c.put(x, top-1, "◢", style{fg: wall})
					c.put(x+1, top-1, "█", style{fg: wall})
				}
			}
		}
	}

	// The sign and the tagline go on last, over the sky.
	drawSign(c, signX, signY, frame, framed)
	lamp := blend(cRed, hex(0x4a1620), pulse(frame, 24))
	if md.busy {
		lamp = blend(cGreen, hex(0x0e4a31), pulse(frame, 12))
	}
	c.put(tx, ty, "●", style{fg: lamp})
	c.put(tx+2, ty, "夜 間 操 業 中", style{fg: blend(cGold, cAmber, pulse(frame, 50)), bold: true})
	c.put(tx, ty+1, "単機能 CLI 工場 ― 欲しい道具を、一言で。", style{fg: cInk})
	c.put(tx, ty+2, md.status, style{fg: cDim})
}

// drawSign paints the neon logo, with a border of chasing bulbs when there
// is room for one.
func drawSign(c *canvas, x, y, frame int, framed bool) {
	lx, ly := x, y
	if framed {
		w, h := logoW+6, 5
		lx, ly = x+3, y+1
		for i := 0; i < w; i++ {
			for j := 0; j < h; j++ {
				c.put(x+i, y+j, " ", style{bg: hex(0x0b0d18)})
			}
		}
		// Walk the border clockwise so the chase runs around the sign.
		p := 0
		bulb := func(bx, by int) {
			col := blend(cAmber, hex(0x2b1d08), 0.75)
			if (p-frame/2)%4 == 0 {
				col = cGold
			}
			c.put(bx, by, "•", style{fg: col, bold: true})
			p++
		}
		for i := 0; i < w; i += 2 {
			bulb(x+i, y)
		}
		for j := 1; j < h-1; j++ {
			bulb(x+w-1, y+j)
		}
		for i := w - 1; i >= 0; i -= 2 {
			bulb(x+i, y+h-1)
		}
		for j := h - 2; j >= 1; j-- {
			bulb(x, y+j)
		}
	}
	// The tubes: a slow colour drift, a highlight that sweeps across, and
	// the occasional flicker real neon has.
	sweep := float64(frame%110) * 0.45
	flicker := 1.0
	if noise(frame/2, 5)%47 == 0 {
		flicker = 0.55
	}
	for j, row := range logo {
		i := 0
		for _, r := range row {
			if r != ' ' {
				t := math.Mod(float64(i)/logoW+float64(frame)*0.006, 1)
				col := gradient(t, cPink, cAmber, cCyan, cPink)
				if d := math.Abs(float64(i+j) - sweep); d < 2.5 {
					col = blend(col, cWhite, 0.85-d*0.3)
				}
				c.put(lx+i, ly+j, string(r), style{fg: blend(cNight, col, flicker), bold: true})
			}
			i++
		}
	}
}

func ifInt(cond bool, a, b int) int {
	if cond {
		return a
	}
	return b
}
