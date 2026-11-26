// mdid: mdid reads CSV from a file or stdin and prints a Markdown table with a deterministic UUID v5 id column computed from each row's content.
package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
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

const usage = `mdid reads CSV from a file or stdin and prints a Markdown table with a deterministic UUID v5 id column computed from each row's content.

Usage:
  mdid [-s] [file]

Examples:
  mdid data.csv
  mdid -s data.csv
  cat data.csv | mdid -s

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

var escaper = strings.NewReplacer("|", "\\|", "\r\n", "<br>", "\n", "<br>", "\r", "<br>")

func uuid5(fields []string) string {
	ns, _ := hex.DecodeString("6ba7b8109dad11d180b400c04fd430c8")
	h := sha1.New()
	h.Write(ns)
	h.Write([]byte(strings.Join(fields, "\x00")))
	sum := h.Sum(nil)[:16]
	sum[6] = (sum[6] & 0x0f) | 0x50
	sum[8] = (sum[8] & 0x3f) | 0x80
	s := hex.EncodeToString(sum)
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}

func esc(fields []string) string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = escaper.Replace(f)
	}
	return strings.Join(out, " | ")
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("mdid", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	short := fs.Bool("s", false, "shorten each id to its first 8 characters")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'mdid -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "mdid: usage: mdid [-s] [file]")
		return 2
	}

	var in io.Reader = stdin
	if fs.NArg() == 1 {
		f, err := os.Open(fs.Arg(0))
		if err != nil {
			fmt.Fprintln(stderr, "mdid: "+err.Error())
			return 1
		}
		defer f.Close()
		in = f
	} else if f, ok := stdin.(*os.File); ok {
		if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
			fmt.Fprintln(stderr, "Run 'mdid -h' for usage.")
			return 2
		}
	}

	r := csv.NewReader(in)
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		fmt.Fprintln(stderr, "mdid: "+err.Error())
		return 1
	}
	if len(recs) == 0 {
		fmt.Fprintln(stderr, "mdid: empty input")
		return 1
	}
	header := recs[0]
	n := len(header)
	var buf bytes.Buffer
	buf.WriteString("| id | " + esc(header) + " |\n")
	seps := make([]string, n+1)
	for i := range seps {
		seps[i] = "---"
	}
	buf.WriteString("| " + strings.Join(seps, " | ") + " |\n")
	for _, rec := range recs[1:] {
		row := make([]string, n)
		copy(row, rec)
		id := uuid5(row)
		if *short {
			id = id[:8]
		}
		buf.WriteString("| " + id + " | " + esc(row) + " |\n")
	}
	stdout.Write(buf.Bytes())
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
