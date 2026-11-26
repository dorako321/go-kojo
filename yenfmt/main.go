// yenfmt: yenfmt rewrites yen amounts found in text into comma, man or kanji notation and leaves all other text unchanged.
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

const usage = `yenfmt rewrites yen amounts found in text into comma, man or kanji notation and leaves all other text unchanged.

Usage:
  yenfmt [-s comma|man|kanji] [file ...]

Examples:
  echo '八万五千円' | yenfmt
  echo '1億2345万円' | yenfmt -s man
  yenfmt -s kanji prices.txt

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

const maxVal = 999999999999999

var re = regexp.MustCompile(`[¥￥][0-9][0-9,]*円?|[0-9零〇一二三四五六七八九壱弐参][0-9,零〇一二三四五六七八九壱弐参十拾百千万億]*円`)
var tok = regexp.MustCompile(`[0-9]+|.`)
var vals = map[rune]int64{'零': 0, '〇': 0, '一': 1, '壱': 1, '二': 2, '弐': 2, '三': 3, '参': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9, '十': 10, '拾': 10, '百': 100, '千': 1000, '万': 1e4, '億': 1e8}
var kn = []rune("〇一二三四五六七八九")

// value returns the amount of a match body and false if the match stays unchanged.
func value(s string) (int64, bool) {
	var total, section, num int64
	set := false
	for _, t := range tok.FindAllString(strings.ReplaceAll(s, ",", ""), -1) {
		u, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			u = vals[[]rune(t)[0]]
		}
		if isNum := err == nil || u < 10; len(t) > 15 || isNum && set {
			return 0, false
		} else if isNum {
			num, set = u, true
			continue
		}
		if u < 1e4 {
			if !set {
				num = 1
			}
			section += num * u
		} else {
			if section += num; section == 0 {
				section = 1
			}
			if section > maxVal/u {
				return 0, false
			}
			total, section = total+section*u, 0
		}
		num, set = 0, false
		if total > maxVal || section > maxVal {
			return 0, false
		}
	}
	total += section + num
	return total, total <= maxVal
}

func comma(n int64) string {
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func f(n int64) string {
	s := ""
	for i, u := range []string{"千", "百", "十", ""} {
		d := n / []int64{1000, 100, 10, 1}[i] % 10
		if d > 1 || d == 1 && u == "" {
			s += string(kn[d])
		}
		if d > 0 {
			s += u
		}
	}
	return s
}

func k(n int64) string {
	if n >= 1e4 {
		return f(n/1e4) + "万" + f(n%1e4)
	}
	return f(n)
}

func format(v int64, style string) string {
	if style == "comma" || v == 0 && style == "man" {
		return comma(v) + "円"
	} else if v == 0 {
		return "零円"
	}
	fn, s := k, ""
	if style == "man" {
		fn = comma
	}
	for i, n := range []int64{v / 1e8, v % 1e8 / 1e4, v % 1e4} {
		if n > 0 {
			s += fn(n) + []string{"億", "万", ""}[i]
		}
	}
	return s + "円"
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("yenfmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	style := fs.String("s", "comma", "output style: comma (12,000円), man (1億2,345万円) or kanji (一万二千円)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'yenfmt -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}
	if *style != "comma" && *style != "man" && *style != "kanji" {
		fmt.Fprintf(stderr, "yenfmt: invalid -s value \"%s\" (use comma, man or kanji)\n", *style)
		return 2
	}
	var data []byte
	var err error
	if fs.NArg() == 0 {
		if fl, ok := stdin.(*os.File); ok {
			if st, e := fl.Stat(); e == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'yenfmt -h' for usage.")
				return 2
			}
		}
		data, err = io.ReadAll(stdin)
	}
	for _, name := range fs.Args() {
		b, e := os.ReadFile(name)
		if data, err = append(data, b...), e; err != nil {
			break
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, "yenfmt: "+err.Error())
		return 1
	}
	fmt.Fprint(stdout, re.ReplaceAllStringFunc(string(data), func(m string) string {
		if v, ok := value(strings.TrimSuffix(strings.TrimLeft(m, "¥￥"), "円")); ok {
			return format(v, *style)
		}
		return m
	}))
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
