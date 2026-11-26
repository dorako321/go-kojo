// youbichk: youbichk finds dates with a written weekday in Japanese text and reports those whose weekday does not match the real calendar.
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
	"time"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `youbichk finds dates with a written weekday in Japanese text and reports those whose weekday does not match the real calendar.

Usage:
  youbichk [-a] [file ...]

Examples:
  youbichk memo.txt
  youbichk -a a.txt b.txt
  echo '令和3年4月5日（火）' | youbichk

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

var eras = map[string][2]int{
	"明治": {1868, 45}, "大正": {1912, 15}, "昭和": {1926, 64}, "平成": {1989, 31}, "令和": {2019, 0},
}

const wdays = "日月火水木金土"

var re = regexp.MustCompile(`(?:(明治|大正|昭和|平成|令和)(元|[0-9]+)|([0-9]{4}))年([0-9]{1,2})月([0-9]{1,2})日[(（]([日月火水木金土])[)）]`)

// check returns the real weekday index and whether the date is valid.
func check(m []string) (int, bool) {
	var year int
	if m[1] != "" {
		e := eras[m[1]]
		n := 1
		if m[2] != "元" {
			var err error
			n, err = strconv.Atoi(m[2])
			if err != nil {
				return 0, false
			}
		}
		if n < 1 || (e[1] > 0 && n > e[1]) {
			return 0, false
		}
		year = e[0] + n - 1
	} else {
		year, _ = strconv.Atoi(m[3])
	}
	mo, _ := strconv.Atoi(m[4])
	d, _ := strconv.Atoi(m[5])
	t := time.Date(year, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || int(t.Month()) != mo || t.Day() != d {
		return 0, false
	}
	return int(t.Weekday()), true
}

func wdName(i int) string {
	return string([]rune(wdays)[i])
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("youbichk", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	all := fs.Bool("a", false, "also print dates whose weekday matches")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'youbichk -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	files := fs.Args()
	if len(files) == 0 {
		if f, ok := stdin.(*os.File); ok {
			if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'youbichk -h' for usage.")
				return 2
			}
		}
	}
	code := 0
	scan := func(r io.Reader, prefix string) error {
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		for ln, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSuffix(line, "\r")
			for _, m := range re.FindAllStringSubmatch(line, -1) {
				date := m[1] + m[2] + m[3] + "年" + m[4] + "月" + m[5] + "日"
				wd := strings.Index(wdays, m[6]) / 3
				real, valid := check(m)
				switch {
				case !valid:
					fmt.Fprintf(stdout, "%s%d: %s は存在しない日付です\n", prefix, ln+1, date)
					code = 1
				case real != wd:
					fmt.Fprintf(stdout, "%s%d: %s は%s曜日（%sと書かれている）\n", prefix, ln+1, date, wdName(real), wdName(wd))
					code = 1
				case *all:
					fmt.Fprintf(stdout, "%s%d: %s は%s曜日（一致）\n", prefix, ln+1, date, wdName(real))
				}
			}
		}
		return nil
	}
	if len(files) == 0 {
		if err := scan(stdin, ""); err != nil {
			fmt.Fprintln(stderr, "youbichk: "+err.Error())
			code = 1
		}
		return code
	}
	for _, name := range files {
		prefix := ""
		if len(files) > 1 {
			prefix = name + ":"
		}
		f, err := os.Open(name)
		if err == nil {
			err = scan(f, prefix)
			f.Close()
		}
		if err != nil {
			fmt.Fprintln(stderr, "youbichk: "+err.Error())
			code = 1
		}
	}
	return code
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
