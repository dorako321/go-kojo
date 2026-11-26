// jsonmd: jsonmd reads a JSON array of objects or NDJSON from a file or stdin and prints a Markdown table whose columns are the union of all keys.
package main

import (
	"bytes"
	"encoding/json"
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

const usage = `jsonmd reads a JSON array of objects or NDJSON from a file or stdin and prints a Markdown table whose columns are the union of all keys.

Usage:
  jsonmd [file]

Examples:
  jsonmd data.json
  jsonmd data.jsonl > table.md
  curl -s example.com/items | jsonmd

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("jsonmd", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'jsonmd -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "usage: jsonmd [file]")
		return 2
	}

	var data []byte
	var err error
	if fs.NArg() == 1 {
		data, err = os.ReadFile(fs.Arg(0))
	} else {
		if f, ok := stdin.(*os.File); ok {
			if st, e := f.Stat(); e == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'jsonmd -h' for usage.")
				return 2
			}
		}
		data, err = io.ReadAll(stdin)
	}
	if err != nil {
		fmt.Fprintln(stderr, "jsonmd: "+err.Error())
		return 1
	}

	out, err := convert(data)
	if err != nil {
		fmt.Fprintln(stderr, "jsonmd: "+err.Error())
		return 1
	}
	io.WriteString(stdout, out)
	return 0
}

func convert(data []byte) (string, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "", errors.New("no data")
	}
	var raws []json.RawMessage
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &raws); err != nil {
			return "", fmt.Errorf("invalid JSON: %v", err)
		}
	} else {
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		for {
			var r json.RawMessage
			err := dec.Decode(&r)
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", fmt.Errorf("invalid JSON: %v", err)
			}
			raws = append(raws, r)
		}
	}

	var cols []string
	seen := map[string]bool{}
	var rows []map[string]string
	for i, r := range raws {
		if r[0] != '{' {
			return "", fmt.Errorf("element %d is not an object", i+1)
		}
		d := json.NewDecoder(bytes.NewReader(r))
		d.Token()
		row := map[string]string{}
		for d.More() {
			t, _ := d.Token()
			key, _ := t.(string)
			var v json.RawMessage
			d.Decode(&v)
			if !seen[key] {
				seen[key] = true
				cols = append(cols, key)
			}
			row[key] = cell(v)
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 || len(cols) == 0 {
		return "", errors.New("no data")
	}

	var b strings.Builder
	hdr := make([]string, len(cols))
	sep := make([]string, len(cols))
	for i, c := range cols {
		hdr[i] = esc(c)
		sep[i] = "---"
	}
	writeRow(&b, hdr)
	writeRow(&b, sep)
	for _, row := range rows {
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = row[c]
		}
		writeRow(&b, cells)
	}
	return b.String(), nil
}

func writeRow(b *strings.Builder, cells []string) {
	b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
}

func cell(v json.RawMessage) string {
	var s string
	switch v[0] {
	case '"':
		json.Unmarshal(v, &s)
	case 'n':
	case '[', '{':
		var buf bytes.Buffer
		json.Compact(&buf, v)
		s = buf.String()
	default:
		s = string(v)
	}
	return esc(s)
}

func esc(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "|", "\\|")
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
