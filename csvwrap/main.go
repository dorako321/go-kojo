// csvwrap: csvwrap reads CSV from a file or stdin and prints a ruled table that fits a given display width, wrapping long cells inside their columns.
package main

import (
	"encoding/csv"
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

const usage = `csvwrap reads CSV from a file or stdin and prints a ruled table that fits a given display width, wrapping long cells inside their columns.

Usage:
  csvwrap [-w width] [file]

Examples:
  csvwrap data.csv
  csvwrap -w 60 data.csv
  cat data.csv | csvwrap -w 40

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func swidth(s string) (w int) {
	for _, r := range s {
		w += runewidth.RuneWidth(r)
	}
	return w
}

// wrapCell splits s at newlines and hard-wraps each part to width cells.
func wrapCell(s string, width int) (lines []string) {
	for _, part := range strings.Split(s, "\n") {
		cur, cw := "", 0
		for _, r := range part {
			rw := runewidth.RuneWidth(r)
			if cw+rw > width && cur != "" {
				lines, cur, cw = append(lines, cur), "", 0
			}
			cur, cw = cur+string(r), cw+rw
		}
		lines = append(lines, cur)
	}
	return lines
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("csvwrap", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	width := fs.Int("w", 80, "total display width of the table, including borders and padding")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'csvwrap -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "csvwrap: usage: csvwrap [-w width] [file]")
		return 2
	}
	in := stdin
	if fs.NArg() == 1 {
		f, err := os.Open(fs.Arg(0))
		if err != nil {
			fmt.Fprintln(stderr, "csvwrap:", err)
			return 1
		}
		defer f.Close()
		in = f
	} else if f, ok := stdin.(*os.File); ok {
		if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
			fmt.Fprintln(stderr, "Run 'csvwrap -h' for usage.")
			return 2
		}
	}
	cr := csv.NewReader(in)
	cr.FieldsPerRecord = -1
	recs, err := cr.ReadAll()
	if err != nil {
		fmt.Fprintln(stderr, "csvwrap:", err)
		return 1
	}
	if len(recs) == 0 {
		return 0
	}
	n := 0
	for _, r := range recs {
		n = max(n, len(r))
	}
	if *width < 5*n+1 {
		fmt.Fprintf(stderr, "csvwrap: width too small for %d columns (need at least %d)\n", n, 5*n+1)
		return 2
	}
	nat, maxNat := make([]int, n), 1
	for i, r := range recs {
		row := make([]string, n)
		for j, c := range r {
			row[j] = strings.ReplaceAll(c, "\r", "")
			for _, p := range strings.Split(row[j], "\n") {
				nat[j] = max(nat[j], swidth(p), 1)
				maxNat = max(maxNat, nat[j])
			}
		}
		recs[i] = row
	}
	avail := *width - (3*n + 1)
	L := 1
	for ; L < maxNat; L++ {
		s := 0
		for _, v := range nat {
			s += min(v, L+1)
		}
		if s > avail {
			break
		}
	}
	widths, rest := make([]int, n), avail
	for j, v := range nat {
		widths[j] = min(v, L)
		rest -= widths[j]
	}
	for j := 0; j < n && rest > 0; j++ {
		if nat[j] > L {
			widths[j]++
			rest--
		}
	}
	border := "+"
	for _, w := range widths {
		border += strings.Repeat("-", w+2) + "+"
	}
	var b strings.Builder
	b.WriteString(border + "\n")
	for i, r := range recs {
		cells, k := make([][]string, n), 1
		for j, c := range r {
			cells[j] = wrapCell(c, widths[j])
			k = max(k, len(cells[j]))
		}
		for l := 0; l < k; l++ {
			b.WriteString("|")
			for j, w := range widths {
				s := ""
				if l < len(cells[j]) {
					s = cells[j][l]
				}
				b.WriteString(" " + s + strings.Repeat(" ", max(w-swidth(s), 0)) + " |")
			}
			b.WriteString("\n")
		}
		if i == 0 {
			b.WriteString(border + "\n")
		}
	}
	b.WriteString(border + "\n")
	io.WriteString(stdout, b.String())
	return 0
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
