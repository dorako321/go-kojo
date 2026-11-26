// hashart: hashart prints a colored 17x9 drunken-bishop randomart and the hex SHA-256 of a file or stdin.
package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `hashart prints a colored 17x9 drunken-bishop randomart and the hex SHA-256 of a file or stdin.

Usage:
  hashart [-n] [file]

Examples:
  hashart photo.jpg
  hashart -n notes.txt
  echo hello | hashart -n

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("hashart", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	noColor := fs.Bool("n", false, "no color: print plain ASCII without escape codes")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'hashart -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "hashart: too many arguments")
		fmt.Fprintln(stderr, "usage: hashart [-n] [file]")
		return 2
	}

	var data []byte
	var err error
	if fs.NArg() == 1 {
		data, err = os.ReadFile(fs.Arg(0))
		if err != nil {
			fmt.Fprintln(stderr, "hashart: "+err.Error())
			return 1
		}
	} else {
		if f, ok := stdin.(*os.File); ok {
			if st, serr := f.Stat(); serr == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'hashart -h' for usage.")
				return 2
			}
		}
		data, err = io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintln(stderr, "hashart: "+err.Error())
			return 1
		}
	}

	sum := sha256.Sum256(data)
	var grid [9][17]int
	x, y := 8, 4
	for _, by := range sum {
		for i := 0; i < 4; i++ {
			b := (by >> (2 * uint(i))) & 3
			if b&1 != 0 {
				x++
			} else {
				x--
			}
			if b&2 != 0 {
				y++
			} else {
				y--
			}
			if x < 0 {
				x = 0
			}
			if x > 16 {
				x = 16
			}
			if y < 0 {
				y = 0
			}
			if y > 8 {
				y = 8
			}
			grid[y][x]++
		}
	}
	ex, ey := x, y

	const ramp = " .o+=*BOX@%&#/^"
	palette := []int{27, 33, 39, 45, 51, 48, 46, 82, 118, 154, 190, 226, 208, 196}
	var sb strings.Builder
	border := "+" + strings.Repeat("-", 17) + "+\n"
	sb.WriteString(border)
	for r := 0; r < 9; r++ {
		sb.WriteByte('|')
		for c := 0; c < 17; c++ {
			n := grid[r][c]
			if n > 14 {
				n = 14
			}
			ch := ramp[n]
			col := 0
			if n > 0 {
				col = palette[n-1]
			}
			if r == ey && c == ex {
				ch, col = 'E', 196
			} else if r == 4 && c == 8 {
				ch, col = 'S', 46
			}
			if ch == ' ' || *noColor {
				sb.WriteByte(ch)
			} else {
				fmt.Fprintf(&sb, "\x1b[38;5;%dm%c\x1b[0m", col, ch)
			}
		}
		sb.WriteString("|\n")
	}
	sb.WriteString(border)
	fmt.Fprintf(&sb, "%x\n", sum)
	fmt.Fprint(stdout, sb.String())
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
