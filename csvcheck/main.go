// csvcheck: csvcheck reads CSV from a file or stdin and reports structural problems line by line, exiting 1 if any are found.
package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `csvcheck reads CSV from a file or stdin and reports structural problems line by line, exiting 1 if any are found.

Usage:
  csvcheck [-k cols] [file]

Examples:
  csvcheck data.csv
  csvcheck -k 1,2 data.csv
  cat data.csv | csvcheck

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("csvcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	kflag := fs.String("k", "", "comma-separated 1-based column numbers used for duplicate row detection (default: whole row)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'csvcheck -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "usage: csvcheck [-k cols] [file]")
		return 2
	}
	var in io.Reader = stdin
	if fs.NArg() == 1 {
		f, err := os.Open(fs.Arg(0))
		if err != nil {
			fmt.Fprintln(stderr, "csvcheck: "+err.Error())
			return 1
		}
		defer f.Close()
		in = f
	} else if f, ok := stdin.(*os.File); ok {
		if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
			fmt.Fprintln(stderr, "Run 'csvcheck -h' for usage.")
			return 2
		}
	}

	r := csv.NewReader(in)
	r.FieldsPerRecord = -1
	printed := false
	out := func(line int, format string, a ...any) {
		printed = true
		fmt.Fprintf(stdout, "%d: %s\n", line, fmt.Sprintf(format, a...))
	}
	fail := func(err error) int {
		fmt.Fprintln(stderr, "csvcheck: "+err.Error())
		return 1
	}

	var keys []int
	hcount := 0
	seenRows := map[uuid.UUID]int{}
	for n := 0; ; n++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(err)
		}
		line, _ := r.FieldPos(0)
		if n == 0 {
			hcount = len(rec)
			if *kflag != "" {
				for _, item := range strings.Split(*kflag, ",") {
					v, e := strconv.Atoi(item)
					if e != nil || v < 1 || v > hcount {
						fmt.Fprintln(stderr, "csvcheck: invalid -k value: "+*kflag)
						return 2
					}
					keys = append(keys, v)
				}
			}
		} else if len(rec) != hcount {
			out(line, "列数が%dでなく%d", hcount, len(rec))
		}
		if n == 0 {
			seen := map[string]bool{}
			for _, c := range rec {
				if seen[c] {
					out(line, "ヘッダ名「%s」が重複", c)
				}
				seen[c] = true
			}
		}
		for i, c := range rec {
			if c != strings.TrimSpace(c) {
				out(line, "列%dのセルの前後に空白", i+1)
			} else if c == "" {
				out(line, "列%dのセルが空", i+1)
			}
		}
		if n > 0 {
			var sel []string
			if *kflag == "" {
				sel = rec
			} else {
				for _, k := range keys {
					if k-1 < len(rec) {
						sel = append(sel, rec[k-1])
					} else {
						sel = append(sel, "")
					}
				}
			}
			id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(strings.Join(sel, "\x1f")))
			if first, ok := seenRows[id]; ok {
				out(line, "%d行目と重複", first)
			} else {
				seenRows[id] = line
			}
		}
	}
	if printed {
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
