// u4scan: u4scan reports characters that break in old MySQL utf8 or Shift_JIS conversion (4-byte UTF-8 characters, variation selectors, zero width joiner) with line and column.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"unicode/utf8"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `u4scan reports characters that break in old MySQL utf8 or Shift_JIS conversion (4-byte UTF-8 characters, variation selectors, zero width joiner) with line and column.

Usage:
  u4scan [-c] [file ...]

Examples:
  u4scan names.txt
  u4scan -c a.csv b.csv
  cat dump.sql | u4scan

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("u4scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	csv := fs.Bool("c", false, "CSV mode: also print the 1-based CSV column number of each finding")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'u4scan -h' for usage.")
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
				fmt.Fprintln(stderr, "Run 'u4scan -h' for usage.")
				return 2
			}
		}
		if scan(stdin, "", *csv, stdout, stderr, "stdin", &failed) {
			failed = true
		}
	} else {
		for _, name := range files {
			prefix := ""
			if len(files) > 1 {
				prefix = name + ":"
			}
			f, err := os.Open(name)
			if err != nil {
				fmt.Fprintf(stderr, "u4scan: %v\n", err)
				failed = true
				continue
			}
			scan(f, prefix, *csv, stdout, stderr, name, &failed)
			f.Close()
		}
	}
	if failed {
		return 1
	}
	return 0
}

// scan reads r line by line and reports findings; it sets *failed on any finding or error.
func scan(r io.Reader, prefix string, csv bool, stdout, stderr io.Writer, name string, failed *bool) bool {
	br := bufio.NewReader(r)
	lineNo := 0
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			lineNo++
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			if n := len(line); n > 0 && line[n-1] == '\r' {
				line = line[:n-1]
			}
			scanLine(line, lineNo, prefix, csv, stdout, failed)
		}
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(stderr, "u4scan: read %s: %v\n", name, err)
				*failed = true
			}
			return false
		}
	}
}

func scanLine(line []byte, lineNo int, prefix string, csv bool, stdout io.Writer, failed *bool) {
	col := 0
	field := 1
	inQ := false
	for len(line) > 0 {
		r, size := utf8.DecodeRune(line)
		line = line[size:]
		col++
		if r == utf8.RuneError && size == 1 {
			continue
		}
		switch {
		case r == '"':
			inQ = !inQ
		case r == ',' && !inQ:
			field++
		}
		if r >= 0x10000 || (r >= 0xFE00 && r <= 0xFE0F) || r == 0x200D {
			*failed = true
			if csv {
				fmt.Fprintf(stdout, "%s%d:%d col %d U+%04X\n", prefix, lineNo, col, field, r)
			} else {
				fmt.Fprintf(stdout, "%s%d:%d U+%04X\n", prefix, lineNo, col, r)
			}
		}
	}
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
