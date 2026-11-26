// jspark: jspark reads a JSON array or NDJSON from a file or stdin, extracts numbers (optionally at a dotted key path) and prints a one-line sparkline with min, max and count.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `jspark reads a JSON array or NDJSON from a file or stdin, extracts numbers (optionally at a dotted key path) and prints a one-line sparkline with min, max and count.

Usage:
  jspark [-k key] [file]

Examples:
  echo '[3,1,4,1,5,9,2,6]' | jspark
  jspark -k ms timings.ndjson
  jspark -k a.b data.json

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func decodeOne(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("trailing data")
	}
	return v, nil
}

func extract(el any, key string) (float64, bool) {
	if key != "" {
		for _, p := range strings.Split(key, ".") {
			m, ok := el.(map[string]any)
			if !ok {
				return 0, false
			}
			el, ok = m[p]
			if !ok {
				return 0, false
			}
		}
	}
	n, ok := el.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(string(n), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("jspark", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	key := fs.String("k", "", "key path of the number in each element, dot-separated for nesting (e.g. a.b)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'jspark -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "jspark: usage: jspark [-k key] [file]")
		return 2
	}
	var data []byte
	var err error
	if fs.NArg() == 1 {
		data, err = os.ReadFile(fs.Arg(0))
		if err != nil {
			var pe *os.PathError
			if errors.As(err, &pe) {
				err = pe.Err
			}
			fmt.Fprintln(stderr, "jspark: "+err.Error())
			return 1
		}
	} else {
		if f, ok := stdin.(*os.File); ok {
			if st, serr := f.Stat(); serr == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'jspark -h' for usage.")
				return 2
			}
		}
		data, err = io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintln(stderr, "jspark: "+err.Error())
			return 1
		}
	}

	var elems []any
	trimmed := bytes.TrimLeft(data, " \t\r\n\f\v")
	if len(trimmed) > 0 && trimmed[0] == '[' {
		v, derr := decodeOne(trimmed)
		if derr != nil {
			fmt.Fprintln(stderr, "jspark: invalid JSON: "+derr.Error())
			return 1
		}
		elems, _ = v.([]any)
	} else {
		for _, line := range bytes.Split(data, []byte("\n")) {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			v, derr := decodeOne(line)
			if derr != nil {
				fmt.Fprintln(stderr, "jspark: invalid JSON: "+derr.Error())
				return 1
			}
			elems = append(elems, v)
		}
	}

	var vals []float64
	for _, el := range elems {
		if f, ok := extract(el, *key); ok {
			vals = append(vals, f)
		}
	}
	if len(vals) == 0 {
		fmt.Fprintln(stderr, "jspark: no numeric values found")
		return 1
	}
	mn, mx := vals[0], vals[0]
	for _, v := range vals {
		mn = math.Min(mn, v)
		mx = math.Max(mx, v)
	}
	bars := []rune("▁▂▃▄▅▆▇█")
	var sb strings.Builder
	for _, v := range vals {
		l := 0
		if mx != mn {
			l = int(math.Floor((v-mn)/(mx-mn)*7 + 0.5))
		}
		if l < 0 {
			l = 0
		}
		if l > 7 {
			l = 7
		}
		sb.WriteRune(bars[l])
	}
	fmt.Fprintf(stdout, "%s  min=%s max=%s n=%d\n", sb.String(),
		strconv.FormatFloat(mn, 'f', -1, 64), strconv.FormatFloat(mx, 'f', -1, 64), len(vals))
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
