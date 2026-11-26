// envchk: envchk checks a .env file (or stdin) line by line for invalid keys, duplicate keys, trailing spaces, full-width characters and unclosed quotes.
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

	"github.com/mattn/go-runewidth"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `envchk checks a .env file (or stdin) line by line for invalid keys, duplicate keys, trailing spaces, full-width characters and unclosed quotes.

Usage:
  envchk [file]

Examples:
  envchk .env
  envchk prod.env
  cat .env | envchk

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func validKey(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		alpha := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
		if alpha {
			continue
		}
		if i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func hasWide(v string) bool {
	var sb strings.Builder
	for _, r := range v {
		if r >= 0x80 {
			sb.WriteRune(r)
		}
	}
	rest := sb.String()
	return runewidth.StringWidth(rest) != utf8.RuneCountInString(rest)
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("envchk", flag.ContinueOnError)
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
		fmt.Fprintln(stderr, "Run 'envchk -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "envchk: usage: envchk [file]")
		return 2
	}
	var data []byte
	var err error
	if fs.NArg() == 1 {
		data, err = os.ReadFile(fs.Arg(0))
	} else {
		if f, ok := stdin.(*os.File); ok {
			if st, e := f.Stat(); e == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'envchk -h' for usage.")
				return 2
			}
		}
		data, err = io.ReadAll(stdin)
	}
	if err != nil {
		fmt.Fprintln(stderr, "envchk: "+err.Error())
		return 1
	}

	runewidth.DefaultCondition.EastAsianWidth = false
	seen := map[string]bool{}
	bad := false
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		text := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(text, "#") {
			continue
		}
		text = strings.TrimPrefix(text, "export ")
		key, val := text, ""
		if idx := strings.Index(text, "="); idx >= 0 {
			key, val = text[:idx], text[idx+1:]
		}
		var probs []string
		ok := validKey(key)
		if !ok {
			probs = append(probs, "不正なキー名")
		} else {
			if seen[key] {
				probs = append(probs, "重複キー")
			}
			seen[key] = true
		}
		if hasWide(val) {
			probs = append(probs, "全角文字")
		}
		quoted := strings.HasPrefix(val, "'") || strings.HasPrefix(val, "\"")
		if !quoted && (strings.HasSuffix(val, " ") || strings.HasSuffix(val, "\t")) {
			probs = append(probs, "末尾空白")
		}
		if quoted && !strings.Contains(val[1:], val[:1]) {
			probs = append(probs, "閉じていない引用符")
		}
		if len(probs) > 0 {
			bad = true
			fmt.Fprintf(stdout, "%d: %s\n", i+1, strings.Join(probs, "と"))
		}
	}
	if bad {
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
