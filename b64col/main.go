// b64col: b64col reads CSV from a file or stdin and Base64-decodes the cells in the chosen columns, writing CSV to stdout.
package main

import (
	"encoding/base64"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"unicode/utf8"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `b64col reads CSV from a file or stdin and Base64-decodes the cells in the chosen columns, writing CSV to stdout.

Usage:
  b64col [-u] -c cols [file]

Examples:
  b64col -c 2 in.csv
  b64col -u -c 1,3 in.csv
  cat in.csv | b64col -c 2

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("b64col", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	cols := fs.String("c", "", "comma-separated 1-based column numbers to decode (required)")
	urlSafe := fs.Bool("u", false, "also accept URL-safe Base64 (- and _)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'b64col -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "b64col: too many arguments")
		return 2
	}
	if *cols == "" {
		fmt.Fprintln(stderr, "b64col: -c is required")
		return 2
	}
	sel := map[int]bool{}
	for _, p := range strings.Split(*cols, ",") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || p[0] == '+' {
			fmt.Fprintln(stderr, "b64col: invalid column list")
			return 2
		}
		sel[n-1] = true
	}

	var in io.Reader = stdin
	if fs.NArg() == 1 {
		f, err := os.Open(fs.Arg(0))
		if err != nil {
			fmt.Fprintln(stderr, "b64col: "+err.Error())
			return 1
		}
		defer f.Close()
		in = f
	} else if f, ok := stdin.(*os.File); ok {
		if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
			fmt.Fprintln(stderr, "Run 'b64col -h' for usage.")
			return 2
		}
	}

	r := csv.NewReader(in)
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		fmt.Fprintln(stderr, "b64col: "+err.Error())
		return 1
	}
	for _, rec := range recs {
		for i := range rec {
			if !sel[i] {
				continue
			}
			rec[i] = decode(rec[i], *urlSafe)
		}
	}
	w := csv.NewWriter(stdout)
	if err := w.WriteAll(recs); err != nil {
		fmt.Fprintln(stderr, "b64col: "+err.Error())
		return 1
	}
	return 0
}

func decode(s string, urlSafe bool) string {
	if s == "" || strings.ContainsAny(s, "\r\n") {
		return s
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil && urlSafe {
		b, err = base64.URLEncoding.DecodeString(s)
	}
	if err != nil || !utf8.Valid(b) {
		return s
	}
	return string(b)
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
