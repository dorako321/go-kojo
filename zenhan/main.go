// zenhan: zenhan normalizes Japanese text by converting full-width ASCII to half-width and half-width katakana to full-width katakana, or lists the lines needing conversion with -r.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"unicode/utf8"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `zenhan normalizes Japanese text by converting full-width ASCII to half-width and half-width katakana to full-width katakana, or lists the lines needing conversion with -r.

Usage:
  zenhan [-r] [file ...]

Examples:
  echo 'ＡＢＣ１２３ ｶﾞｷﾞ' | zenhan
  zenhan notes.txt > normalized.txt
  zenhan -r a.txt b.txt

Flags:
`

const kana = "。「」、・ヲァィゥェォャュョッーアイウエオカキクケコサシスセソタチツテトナニヌネノハヒフヘホマミムメモヤユヨラリルレロワン゛゜"

type prefixWriter struct{ w io.Writer }

func (p prefixWriter) Write(b []byte) (int, error) {
	if _, err := io.WriteString(p.w, "zenhan: "); err != nil {
		return 0, err
	}
	return p.w.Write(b)
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("zenhan", flag.ContinueOnError)
	fs.SetOutput(prefixWriter{stderr})
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	report := fs.Bool("r", false, "report lines that would change as 'lineNumber: original line' instead of converting")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'zenhan -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	files := fs.Args()
	if len(files) == 0 {
		if f, ok := stdin.(*os.File); ok {
			if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'zenhan -h' for usage.")
				return 2
			}
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "zenhan: cannot read stdin: %v\n", err)
			return 1
		}
		process(string(data), *report, stdout)
		return 0
	}
	code := 0
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			reason := err
			var pe *os.PathError
			if errors.As(err, &pe) {
				reason = pe.Err
			}
			fmt.Fprintf(stderr, "zenhan: cannot read %s: %v\n", name, reason)
			code = 1
			continue
		}
		process(string(data), *report, stdout)
	}
	return code
}

func process(text string, report bool, out io.Writer) {
	if text == "" {
		return
	}
	if !report {
		io.WriteString(out, convert(text))
		return
	}
	lines := strings.Split(text, "\n")
	if strings.HasSuffix(text, "\n") {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		if convert(l) != l {
			fmt.Fprintf(out, "%d: %s\n", i+1, l)
		}
	}
}

func half(r rune) (rune, bool) {
	if r < 0xFF61 || r > 0xFF9F {
		return 0, false
	}
	return []rune(kana)[r-0xFF61], true
}

func convert(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			b.WriteByte(s[i])
			i++
			continue
		}
		i += n
		switch {
		case r >= 0xFF01 && r <= 0xFF5E:
			b.WriteRune(r - 0xFEE0)
		case r == 0x3000:
			b.WriteByte(' ')
		default:
			f, ok := half(r)
			if !ok {
				b.WriteRune(r)
				continue
			}
			next, _ := utf8.DecodeRuneInString(s[i:])
			voiceable := (r >= 0xFF76 && r <= 0xFF84) || (r >= 0xFF8A && r <= 0xFF8E) || r == 0xFF73
			if next == 0xFF9E && voiceable {
				if r == 0xFF73 {
					f = 0x30F4
				} else {
					f++
				}
				i += 3
			} else if next == 0xFF9F && r >= 0xFF8A && r <= 0xFF8E {
				f += 2
				i += 3
			}
			b.WriteRune(f)
		}
	}
	return b.String()
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
