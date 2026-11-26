// wrapal: wrapal wraps text from a file or stdin to a display width and aligns each line left, right, center or justified.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/mattn/go-runewidth"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `wrapal wraps text from a file or stdin to a display width and aligns each line left, right, center or justified.

Usage:
  wrapal [-w width] [-a left|right|center|justify] [file]

Examples:
  wrapal -w 30 notes.txt
  wrapal -w 40 -a justify < article.txt
  echo "hello world" | wrapal -w 20 -a center

Flags:
`

// unit is one wrapping unit; sp is 1 when a source space precedes it.
type unit struct {
	s     string
	w, sp int
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("wrapal", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	width := fs.Int("w", 40, "target display width (full-width chars count 2)")
	mode := fs.String("a", "left", "alignment: left, right, center or justify")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'wrapal -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "wrapal: too many arguments")
		return 2
	}
	if *width < 1 {
		fmt.Fprintln(stderr, "wrapal: invalid width")
		return 2
	}
	if m := *mode; m != "left" && m != "right" && m != "center" && m != "justify" {
		fmt.Fprintf(stderr, "wrapal: invalid alignment: %s\n", m)
		return 2
	}
	var data []byte
	var err error
	if fs.NArg() == 1 {
		data, err = os.ReadFile(fs.Arg(0))
	} else {
		if f, ok := stdin.(*os.File); ok {
			if st, e := f.Stat(); e == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'wrapal -h' for usage.")
				return 2
			}
		}
		data, err = io.ReadAll(stdin)
	}
	if err != nil {
		fmt.Fprintf(stderr, "wrapal: %v\n", err)
		return 1
	}
	var para strings.Builder
	last, sep := rune(0), ""
	for _, ln := range append(strings.Split(string(data), "\n"), "") {
		ln = strings.TrimSuffix(ln, "\r")
		if strings.Trim(ln, " \t") != "" {
			rs := []rune(ln)
			if para.Len() > 0 && runewidth.RuneWidth(last) != 2 && runewidth.RuneWidth(rs[0]) != 2 {
				para.WriteByte(' ')
			}
			para.WriteString(ln)
			last = rs[len(rs)-1]
		} else if para.Len() > 0 {
			fmt.Fprint(stdout, sep)
			lines := fill(splitUnits(para.String(), *width), *width)
			for i, l := range lines {
				fmt.Fprintln(stdout, align(l, *width, *mode, i == len(lines)-1))
			}
			sep = "\n"
			para.Reset()
		}
	}
	return 0
}

func splitUnits(text string, w int) (us []unit) {
	cur, cw, sp := []byte(nil), 0, 0
	emit := func() {
		if len(cur) > 0 {
			us = append(us, unit{string(cur), cw, sp})
			cur, cw, sp = cur[:0], 0, 0
		}
	}
	for _, r := range text + " " {
		rw, blank := runewidth.RuneWidth(r), r == ' ' || r == '\t'
		if blank || rw == 2 || cw+rw > w {
			emit()
		}
		if blank {
			sp = 1
		} else if cur, cw = append(cur, string(r)...), cw+rw; rw == 2 {
			emit()
		}
	}
	return us
}

func lineWidth(l []unit) int {
	t := -l[0].sp
	for _, u := range l {
		t += u.w + u.sp
	}
	return t
}

func fill(us []unit, w int) (lines [][]unit) {
	cur, cw := []unit{us[0]}, us[0].w
	for _, u := range us[1:] {
		if cw+u.sp+u.w <= w {
			cur, cw = append(cur, u), cw+u.sp+u.w
			continue
		}
		nl := []unit{u}
		for strings.Contains("、。，．）」』】〕！？", nl[0].s) && len(cur) > 1 {
			nl, cur = append([]unit{cur[len(cur)-1]}, nl...), cur[:len(cur)-1]
		}
		lines, cur, cw = append(lines, cur), nl, lineWidth(nl)
	}
	return append(lines, cur)
}

func align(l []unit, w int, mode string, last bool) string {
	pad, extra, gaps := w-lineWidth(l), make([]int, len(l)), []int{}
	extra[0] = -l[0].sp
	for pass := 0; pass < 2 && len(gaps) == 0 && mode == "justify" && !last && pad > 0; pass++ {
		for i := 1; i < len(l); i++ {
			if l[i].sp+pass > 0 {
				gaps = append(gaps, i)
			}
		}
	}
	for k, g := range gaps {
		extra[g] = (pad + len(gaps) - 1 - k) / len(gaps) // pad/n, +1 for the first pad%n
	}
	var sb strings.Builder
	for i, u := range l {
		sb.WriteString(strings.Repeat(" ", u.sp+extra[i]) + u.s)
	}
	switch {
	case pad > 0 && mode == "right":
		return strings.Repeat(" ", pad) + sb.String()
	case pad > 0 && mode == "center":
		return strings.Repeat(" ", pad/2) + sb.String() + strings.Repeat(" ", pad-pad/2)
	}
	return sb.String()
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
