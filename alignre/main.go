// alignre: alignre pads lines with spaces so the first match of a regex starts at the same display column on every line.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/mattn/go-runewidth"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `alignre pads lines with spaces so the first match of a regex starts at the same display column on every line.

Usage:
  alignre -e REGEX [-p N] [file...]

Examples:
  alignre -e = config.txt
  alignre -e '#' -p 2 main.go
  printf 'a=1\nlong=2\n' | alignre -e =

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func splitLines(data string) []string {
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(data, "\n"), "\n")
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("alignre", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	expr := fs.String("e", "", "regular expression (Go syntax) to align on (required)")
	pad := fs.Int("p", 0, "minimum number of spaces inserted before the match")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'alignre -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	if *expr == "" {
		fmt.Fprintln(stderr, "alignre: -e is required")
		return 2
	}
	if *pad < 0 {
		fmt.Fprintln(stderr, "alignre: -p must be >= 0")
		return 2
	}
	re, err := regexp.Compile(*expr)
	if err != nil {
		fmt.Fprintln(stderr, "alignre: invalid regex: "+err.Error())
		return 1
	}

	var lines []string
	files := fs.Args()
	if len(files) == 0 {
		if f, ok := stdin.(*os.File); ok {
			if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'alignre -h' for usage.")
				return 2
			}
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintln(stderr, "alignre: "+err.Error())
			return 1
		}
		lines = splitLines(string(data))
	} else {
		for _, name := range files {
			data, err := os.ReadFile(name)
			if err != nil {
				fmt.Fprintln(stderr, "alignre: "+err.Error())
				return 1
			}
			lines = append(lines, splitLines(string(data))...)
		}
	}

	starts := make([]int, len(lines))
	widths := make([]int, len(lines))
	maxW := 0
	for i, l := range lines {
		loc := re.FindStringIndex(l)
		if loc == nil {
			starts[i] = -1
			continue
		}
		starts[i] = loc[0]
		widths[i] = runewidth.StringWidth(l[:loc[0]])
		if widths[i] > maxW {
			maxW = widths[i]
		}
	}
	target := maxW + *pad

	var sb strings.Builder
	for i, l := range lines {
		if starts[i] >= 0 {
			sb.WriteString(l[:starts[i]])
			sb.WriteString(strings.Repeat(" ", target-widths[i]))
			sb.WriteString(l[starts[i]:])
		} else {
			sb.WriteString(l)
		}
		sb.WriteByte('\n')
	}
	io.WriteString(stdout, sb.String())
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
