// rainbowqr: rainbowqr shows the given text as a QR code in the terminal with a rainbow color animation.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	qrcode "github.com/skip2/go-qrcode"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `rainbowqr shows the given text as a QR code in the terminal with a rainbow color animation.

Usage:
  rainbowqr [-d seconds] [text]

Examples:
  rainbowqr https://example.com
  rainbowqr -d 5 "hello world"
  echo hello | rainbowqr -d 0

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rainbowqr", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	dur := fs.Float64("d", 3, "animation length in seconds (0 disables the animation)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'rainbowqr -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "rainbowqr: too many arguments")
		return 2
	}
	if math.IsNaN(*dur) || *dur < 0 {
		fmt.Fprintln(stderr, "rainbowqr: invalid duration")
		return 2
	}
	var text string
	if fs.NArg() == 1 {
		text = fs.Arg(0)
	} else {
		if f, ok := stdin.(*os.File); ok {
			if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'rainbowqr -h' for usage.")
				return 2
			}
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintln(stderr, "rainbowqr: "+err.Error())
			return 1
		}
		text = string(b)
	}
	text = strings.TrimRight(text, "\r\n")
	if text == "" {
		fmt.Fprintln(stderr, "rainbowqr: no text given")
		fmt.Fprintln(stderr, "Usage: rainbowqr [-d seconds] [text]")
		return 2
	}

	qr, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		fmt.Fprintln(stderr, "rainbowqr: "+err.Error())
		return 1
	}

	isTerm := false
	if f, ok := stdout.(*os.File); ok {
		isTerm = isatty.IsTerminal(f.Fd())
	}
	if !isTerm || *dur == 0 {
		fmt.Fprint(stdout, qr.ToSmallString(false))
		return 0
	}

	animate(stdout, qr.Bitmap(), *dur)
	return 0
}

func animate(w io.Writer, bm [][]bool, dur float64) {
	fmt.Fprint(w, "\x1b[?25l")
	defer fmt.Fprint(w, "\x1b[0m\x1b[?25h")
	h := len(bm)
	width := 0
	if h > 0 {
		width = len(bm[0])
	}
	lines := (h + 1) / 2
	n := int(dur/0.03 + 0.5)
	if n < 1 {
		n = 1
	}
	half := float64(n) / 2
	for frame := 0; frame < n; frame++ {
		front := width + 1
		if frame != n-1 && float64(frame) < half {
			front = int(float64(width) * float64(frame) / half)
		}
		var sb strings.Builder
		if frame > 0 {
			fmt.Fprintf(&sb, "\x1b[%dA\r", lines)
		}
		for l := 0; l < lines; l++ {
			y := 2 * l
			for x := 0; x < width; x++ {
				top := bm[y][x]
				bot := y+1 < h && bm[y+1][x]
				ch := " "
				switch {
				case top && bot:
					ch = "█"
				case top:
					ch = "▀"
				case bot:
					ch = "▄"
				}
				if x > front || ch == " " {
					sb.WriteString(" ")
					continue
				}
				if x == front {
					sb.WriteString("\x1b[38;2;255;255;255m" + ch)
					continue
				}
				r, g, b := hue(float64((x+y)*6 + frame*12))
				fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm%s", r, g, b, ch)
			}
			sb.WriteString("\x1b[0m\n")
		}
		fmt.Fprint(w, sb.String())
		if frame < n-1 {
			time.Sleep(30 * time.Millisecond)
		}
	}
}

func hue(deg float64) (int, int, int) {
	deg = math.Mod(deg, 360)
	hp := deg / 60
	x := 1 - math.Abs(math.Mod(hp, 2)-1)
	var r, g, b float64
	switch int(hp) {
	case 0:
		r, g = 1, x
	case 1:
		r, g = x, 1
	case 2:
		g, b = 1, x
	case 3:
		g, b = x, 1
	case 4:
		r, b = x, 1
	default:
		r, b = 1, x
	}
	return int(r * 255), int(g * 255), int(b * 255)
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "dev"
}
