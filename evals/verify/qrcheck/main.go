// qrcheck runs a gqr-style program and decodes what it prints, the way a
// phone camera would. It exits 0 only when every code reads back as the
// message that went in.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: qrcheck <binary>")
		os.Exit(2)
	}
	checks := []struct {
		args  []string
		stdin string
		want  string
	}{
		{[]string{"-m", "https://example.com/kojo?id=42&x=y"}, "", "https://example.com/kojo?id=42&x=y"},
		{nil, "hello kojo\n", "hello kojo"},
		{[]string{"-m", "日本語のメッセージ"}, "", "日本語のメッセージ"},
	}
	failed := false
	for _, c := range checks {
		cmd := exec.Command(os.Args[1], c.args...)
		cmd.Stdin = strings.NewReader(c.stdin)
		out, err := cmd.Output()
		if err != nil {
			fmt.Printf("FAIL %q: %v\n", c.want, err)
			failed = true
			continue
		}
		got, err := decode(out)
		if err != nil {
			fmt.Printf("FAIL %q: %v\n", c.want, err)
			failed = true
		} else if got != c.want {
			fmt.Printf("FAIL %q: decoded %q\n", c.want, got)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
	fmt.Println("ok")
}

// decode turns terminal output into a bitmap and reads the QR code in it.
func decode(out []byte) (string, error) {
	var rows [][]bool // true = dark module
	for n, line := range bytes.Split(bytes.TrimRight(out, "\n"), []byte("\n")) {
		cells, err := cells(line)
		if err != nil {
			return "", fmt.Errorf("line %d: %v", n+1, err)
		}
		if len(cells)%2 != 0 {
			return "", fmt.Errorf("line %d: %d painted cells, a module must be two cells wide", n+1, len(cells))
		}
		row := make([]bool, len(cells)/2)
		for i := range row {
			if cells[2*i] != cells[2*i+1] {
				return "", fmt.Errorf("line %d: module %d is half light, half dark", n+1, i)
			}
			row[i] = cells[2*i]
		}
		rows = append(rows, row)
	}
	if len(rows) < 21 {
		return "", fmt.Errorf("only %d rows", len(rows))
	}
	for n, r := range rows {
		if len(r) != len(rows) {
			return "", fmt.Errorf("not square: line %d has %d modules, there are %d lines", n+1, len(r), len(rows))
		}
	}

	const scale = 8
	img := image.NewGray(image.Rect(0, 0, len(rows)*scale, len(rows)*scale))
	for y, r := range rows {
		for x, dark := range r {
			c := color.Gray{Y: 255}
			if dark {
				c.Y = 0
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.SetGray(x*scale+dx, y*scale+dy, c)
				}
			}
		}
	}
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", err
	}
	res, err := qrcode.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		return "", fmt.Errorf("not readable as a QR code: %v", err)
	}
	return res.GetText(), nil
}

// cells walks one line, tracking the ANSI background colour, and returns
// true for each space painted dark and false for each painted light.
func cells(line []byte) ([]bool, error) {
	const (
		unset = iota
		light
		dark
	)
	bg := unset
	var out []bool
	for i := 0; i < len(line); i++ {
		switch {
		case line[i] == 0x1b && i+1 < len(line) && line[i+1] == '[':
			end := bytes.IndexByte(line[i:], 'm')
			if end < 0 {
				return nil, fmt.Errorf("unterminated escape sequence")
			}
			params := strings.Split(string(line[i+2:i+end]), ";")
			for p := 0; p < len(params); p++ {
				switch n, _ := strconv.Atoi(params[p]); {
				case n == 0, n == 49:
					bg = unset
				case n == 40:
					bg = dark
				case n == 47, n == 107:
					bg = light
				case n == 48 && p+2 < len(params) && params[p+1] == "5":
					v, _ := strconv.Atoi(params[p+2])
					bg = light
					if v == 0 || v == 16 || (v >= 232 && v < 244) {
						bg = dark
					}
					p += 2
				}
			}
			i += end
		case line[i] == ' ':
			if bg == unset {
				return nil, fmt.Errorf("a space at byte %d has no background colour, so its colour depends on the terminal theme", i)
			}
			out = append(out, bg == dark)
		default:
			return nil, fmt.Errorf("unexpected byte %q at %d", line[i], i)
		}
	}
	return out, nil
}
