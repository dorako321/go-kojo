// colhash: colhash prints a Markdown table of per-column SHA-256 prefixes for one CSV file, or compares them between two CSV files.
package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime/debug"
	"strings"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `colhash prints a Markdown table of per-column SHA-256 prefixes for one CSV file, or compares them between two CSV files.

Usage:
  colhash [-n digits] file1.csv [file2.csv]

Examples:
  colhash data.csv
  colhash -n 12 old.csv new.csv

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

type column struct {
	name string
	hash string
}

func loadFile(path string, n int) ([]column, error) {
	f, err := os.Open(path)
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			return nil, pe.Err
		}
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, nil
	}
	seen := map[string]int{}
	var cols []column
	for i, h := range recs[0] {
		seen[h]++
		name := h
		if seen[h] > 1 {
			name = fmt.Sprintf("%s#%d", h, seen[h])
		}
		hs := sha256.New()
		for _, row := range recs[1:] {
			if i < len(row) {
				io.WriteString(hs, row[i])
			}
		}
		sum := hex.EncodeToString(hs.Sum(nil))
		cols = append(cols, column{name, sum[:n]})
	}
	return cols, nil
}

func esc(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("colhash", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	n := fs.Int("n", 8, "number of hex characters of each hash (1 to 64)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'colhash -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	files := fs.Args()
	if len(files) < 1 || len(files) > 2 {
		fmt.Fprintln(stderr, "colhash: expected one or two CSV files")
		fmt.Fprintln(stderr, "Usage: colhash [-n digits] file1.csv [file2.csv]")
		return 2
	}
	if *n < 1 || *n > 64 {
		fmt.Fprintln(stderr, "colhash: -n must be between 1 and 64")
		return 2
	}
	var sets [][]column
	for _, p := range files {
		cols, err := loadFile(p, *n)
		if err != nil {
			fmt.Fprintf(stderr, "colhash: %s: %v\n", p, err)
			return 1
		}
		sets = append(sets, cols)
	}
	var b strings.Builder
	if len(sets) == 1 {
		b.WriteString("| Column | Hash |\n| --- | --- |\n")
		for _, c := range sets[0] {
			fmt.Fprintf(&b, "| %s | %s |\n", esc(c.name), c.hash)
		}
	} else {
		b.WriteString("| Column | File1 hash | File2 hash | Status |\n| --- | --- | --- | --- |\n")
		m2 := map[string]string{}
		for _, c := range sets[1] {
			m2[c.name] = c.hash
		}
		m1 := map[string]bool{}
		for _, c := range sets[0] {
			m1[c.name] = true
			h2, ok := m2[c.name]
			st := "same"
			if !ok {
				h2, st = "-", "only-a"
			} else if h2 != c.hash {
				st = "diff"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", esc(c.name), c.hash, h2, st)
		}
		for _, c := range sets[1] {
			if !m1[c.name] {
				fmt.Fprintf(&b, "| %s | - | %s | only-b |\n", esc(c.name), c.hash)
			}
		}
	}
	io.WriteString(stdout, b.String())
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
