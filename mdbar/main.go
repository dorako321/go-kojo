// mdbar: mdbar reads CSV from a file or stdin and prints a Markdown table with an extra bar column for one numeric column.
package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `mdbar reads CSV from a file or stdin and prints a Markdown table with an extra bar column for one numeric column.

Usage:
  mdbar -c column [-w width] [file]

Examples:
  mdbar -c sales data.csv
  mdbar -c 2 -w 10 data.csv
  cat data.csv | mdbar -c qty -w 30

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

var escaper = strings.NewReplacer("|", "\\|", "\r\n", " ", "\n", " ", "\r", " ")

func parseNum(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mdbar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	col := fs.String("c", "", "column to graph: header name or 1-based number (required)")
	width := fs.Int("w", 20, "bar width for the largest value (1 or more)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'mdbar -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	if fs.NArg() > 1 || *col == "" || *width < 1 {
		fmt.Fprintln(stderr, "mdbar: -c is required, -w must be 1 or more, and at most one file may be given")
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
		return 2
	}

	in := stdin
	if fs.NArg() == 1 {
		f, err := os.Open(fs.Arg(0))
		if err != nil {
			fmt.Fprintln(stderr, "mdbar: "+err.Error())
			return 1
		}
		defer f.Close()
		in = f
	} else if f, ok := stdin.(*os.File); ok {
		if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
			fmt.Fprintln(stderr, "Run 'mdbar -h' for usage.")
			return 2
		}
	}

	recs, err := csv.NewReader(in).ReadAll()
	if err != nil {
		fmt.Fprintln(stderr, "mdbar: "+err.Error())
		return 1
	}
	if len(recs) == 0 {
		fmt.Fprintln(stderr, "mdbar: empty input")
		return 1
	}
	header, idx := recs[0], -1
	for i, h := range header {
		if h == *col {
			idx = i
			break
		}
	}
	if n, err := strconv.Atoi(*col); idx < 0 && err == nil && n >= 1 && n <= len(header) {
		idx = n - 1
	}
	if idx < 0 {
		fmt.Fprintln(stderr, "mdbar: unknown column: "+*col)
		return 1
	}

	runewidth.DefaultCondition.EastAsianWidth = false
	runewidth.DefaultCondition.CreateLUT()

	mx := 0.0
	for _, r := range recs[1:] {
		if v, ok := parseNum(r[idx]); ok && v > mx {
			mx = v
		}
	}

	rows := make([][]string, len(recs))
	for i, r := range recs {
		row := make([]string, 0, len(r)+1)
		for _, c := range r {
			row = append(row, escaper.Replace(c))
		}
		bar := "bar"
		if i > 0 {
			bar = ""
			if v, ok := parseNum(r[idx]); ok && mx > 0 && v > 0 {
				bar = strings.Repeat("█", int(math.Round(v/mx*float64(*width))))
			}
		}
		rows[i] = append(row, bar)
	}

	n := len(header) + 1
	wd := func(c string, j int) int {
		if j == n-1 {
			return utf8.RuneCountInString(c)
		}
		return runewidth.StringWidth(c)
	}
	ws := make([]int, n)
	for j := range ws {
		ws[j] = 3
	}
	for _, row := range rows {
		for j, c := range row {
			if w := wd(c, j); w > ws[j] {
				ws[j] = w
			}
		}
	}

	line := func(cells []string) {
		fmt.Fprint(stdout, "|")
		for j, c := range cells {
			fmt.Fprint(stdout, " "+c+strings.Repeat(" ", ws[j]-wd(c, j))+" |")
		}
		fmt.Fprintln(stdout)
	}
	line(rows[0])
	sep := make([]string, n)
	for j := range sep {
		sep[j] = strings.Repeat("-", ws[j])
	}
	line(sep)
	for _, row := range rows[1:] {
		line(row)
	}
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
