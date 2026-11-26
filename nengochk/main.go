// nengochk: nengochk checks Japanese era and Western year pairs such as 令和3年（2021年） in text and reports mismatches.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `nengochk checks Japanese era and Western year pairs such as 令和3年（2021年） in text and reports mismatches.

Usage:
  nengochk [file ...]

Examples:
  echo '平成31年（2020年）' | nengochk
  nengochk a.txt b.txt
  nengochk notes.txt

Flags:
`

var pairRe = regexp.MustCompile(`(明治|大正|昭和|平成|令和)(元|[0-9０-９]+)年?[(（]([0-9０-９]+)年?[)）]年?`)

type era struct {
	name  string
	start int
	max   int // 0 = no maximum
}

var eras = []era{
	{"明治", 1868, 45},
	{"大正", 1912, 15},
	{"昭和", 1926, 64},
	{"平成", 1989, 31},
	{"令和", 2019, 0},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func toNum(s string) int {
	if s == "元" {
		return 1
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '０' && r <= '９' {
			r = r - '０' + '0'
		}
		b.WriteRune(r)
	}
	n, err := strconv.Atoi(b.String())
	if err != nil {
		return 999999999
	}
	return n
}

func reading(x int) string {
	if x < 1868 {
		return ""
	}
	e := eras[0]
	for _, c := range eras {
		if x >= c.start {
			e = c
		}
	}
	return fmt.Sprintf("（%s%d年=%d年）", e.name, x-e.start+1, x)
}

func check(name string, data string, prefix bool, stdout io.Writer) bool {
	bad := false
	for i, line := range strings.Split(data, "\n") {
		line = strings.TrimSuffix(line, "\r")
		for _, m := range pairRe.FindAllStringSubmatch(line, -1) {
			var e era
			for _, c := range eras {
				if c.name == m[1] {
					e = c
				}
			}
			y := toNum(m[2])
			x := toNum(m[3])
			w := e.start + y - 1
			out := ""
			if y < 1 || (e.max > 0 && y > e.max) {
				out = fmt.Sprintf("%s%d年は範囲外%s", e.name, y, reading(x))
			} else if w != x {
				out = fmt.Sprintf("%s%d年=%d年%s", e.name, y, w, reading(x))
			} else {
				continue
			}
			bad = true
			p := ""
			if prefix {
				p = name + ":"
			}
			fmt.Fprintf(stdout, "%s%d: %s → %s\n", p, i+1, m[0], out)
		}
	}
	return bad
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("nengochk", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'nengochk -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	files := fs.Args()
	failed := false
	if len(files) == 0 {
		if f, ok := stdin.(*os.File); ok {
			if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'nengochk -h' for usage.")
				return 2
			}
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintln(stderr, "nengochk: "+err.Error())
			return 1
		}
		if check("", string(b), false, stdout) {
			failed = true
		}
	} else {
		for _, name := range files {
			b, err := os.ReadFile(name)
			if err != nil {
				fmt.Fprintln(stderr, "nengochk: "+err.Error())
				failed = true
				continue
			}
			if check(name, string(b), len(files) > 1, stdout) {
				failed = true
			}
		}
	}
	if failed {
		return 1
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
