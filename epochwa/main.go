// epochwa: epochwa copies text from files or stdin and appends a Japanese-era date-time in full-width parentheses after every 10-digit or 13-digit UNIX timestamp.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"runtime/debug"
	"strconv"
	"time"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `epochwa copies text from files or stdin and appends a Japanese-era date-time in full-width parentheses after every 10-digit or 13-digit UNIX timestamp.

Usage:
  epochwa [-z hours] [file ...]

Examples:
  echo "ts=1735689600" | epochwa
  epochwa -z 0 access.log
  epochwa -z 5.5 a.txt b.txt

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

type era struct {
	name  string
	start int
	year  int
}

var eras = []era{
	{"明治", 18680125, 1868},
	{"大正", 19120730, 1912},
	{"昭和", 19261225, 1926},
	{"平成", 19890108, 1989},
	{"令和", 20190501, 2019},
}

// format returns the era text for the timestamp, or "" if none applies.
func format(secs, offset int64) string {
	t := time.Unix(secs+offset, 0).UTC()
	key := t.Year()*10000 + int(t.Month())*100 + t.Day()
	idx := -1
	for i, e := range eras {
		if e.start <= key {
			idx = i
		}
	}
	if idx < 0 {
		return ""
	}
	e := eras[idx]
	ys := "元"
	if y := t.Year() - e.year + 1; y != 1 {
		ys = strconv.Itoa(y)
	}
	return fmt.Sprintf("%s%s年%d月%d日 %d:%02d:%02d", e.name, ys, int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
}

func convert(in []byte, offset int64) []byte {
	var out bytes.Buffer
	i := 0
	for i < len(in) {
		c := in[i]
		if c < '0' || c > '9' {
			out.WriteByte(c)
			i++
			continue
		}
		j := i
		for j < len(in) && in[j] >= '0' && in[j] <= '9' {
			j++
		}
		run := in[i:j]
		out.Write(run)
		if n := len(run); n == 10 || n == 13 {
			v, err := strconv.ParseInt(string(run), 10, 64)
			if err == nil {
				if n == 13 {
					v /= 1000
				}
				if s := format(v, offset); s != "" {
					out.WriteString("（" + s + "）")
				}
			}
		}
		i = j
	}
	return out.Bytes()
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("epochwa", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	zs := fs.String("z", "9", "timezone offset from UTC in hours (-12 to 14, fractions allowed)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'epochwa -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	z, err := strconv.ParseFloat(*zs, 64)
	if err != nil || math.IsNaN(z) || z < -12 || z > 14 {
		fmt.Fprintln(stderr, "epochwa: invalid -z value")
		return 2
	}
	offset := int64(math.Round(z * 3600))

	var data []byte
	if fs.NArg() == 0 {
		if f, ok := stdin.(*os.File); ok {
			if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'epochwa -h' for usage.")
				return 2
			}
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintln(stderr, "epochwa: "+err.Error())
			return 1
		}
		data = b
	} else {
		for _, name := range fs.Args() {
			b, err := os.ReadFile(name)
			if err != nil {
				fmt.Fprintln(stderr, "epochwa: "+err.Error())
				return 1
			}
			data = append(data, b...)
		}
	}
	if _, err := stdout.Write(convert(data, offset)); err != nil {
		fmt.Fprintln(stderr, "epochwa: "+err.Error())
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
