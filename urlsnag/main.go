// urlsnag: urlsnag finds http:// and https:// URLs in text and reports those with unencoded non-ASCII characters or stray trailing punctuation, with line:column and a suggested fix.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"unicode"
	"unicode/utf8"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `urlsnag finds http:// and https:// URLs in text and reports those with unencoded non-ASCII characters or stray trailing punctuation, with line:column and a suggested fix.

Usage:
  urlsnag [file ...]

Examples:
  urlsnag notes.txt
  urlsnag a.md b.md
  echo 'see https://example.com/資料。' | urlsnag

Flags:
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

const punct = "。、，．）」』】〉》］｝！？"

func isP(r rune) bool { return strings.ContainsRune(punct, r) }

func hasPrefixAt(rs []rune, i int, p string) bool {
	pr := []rune(p)
	if i+len(pr) > len(rs) {
		return false
	}
	for k, c := range pr {
		if rs[i+k] != c {
			return false
		}
	}
	return true
}

func encode(rs []rune) string {
	var sb strings.Builder
	buf := make([]byte, 4)
	for _, r := range rs {
		if r < 128 {
			sb.WriteRune(r)
			continue
		}
		n := utf8.EncodeRune(buf, r)
		for _, b := range buf[:n] {
			fmt.Fprintf(&sb, "%%%02X", b)
		}
	}
	return sb.String()
}

func scan(text, prefix string, stdout io.Writer) bool {
	found := false
	lines := strings.Split(text, "\n")
	for ln, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		rs := []rune(line)
		i := 0
		for i < len(rs) {
			if !hasPrefixAt(rs, i, "http://") && !hasPrefixAt(rs, i, "https://") {
				i++
				continue
			}
			j := i
			for j < len(rs) {
				c := rs[j]
				if unicode.IsSpace(c) || c == '"' || c == '<' || c == '>' || (c >= 0x3040 && c <= 0x309F) {
					break
				}
				if isP(c) {
					for j < len(rs) && isP(rs[j]) {
						j++
					}
					break
				}
				j++
			}
			url := rs[i:j]
			k := len(url)
			for k > 0 && (isP(url[k-1]) || url[k-1] == '.' || url[k-1] == ',') {
				k--
			}
			body, trailer := url[:k], url[k:]
			typ := ""
			hasP := false
			for _, c := range trailer {
				if isP(c) {
					hasP = true
				}
			}
			if hasP {
				typ = "全角句読点混入"
			} else if len(trailer) > 0 {
				typ = "末尾句読点"
			} else {
				for _, c := range body {
					if c > 127 {
						typ = "未エンコード非ASCII"
						break
					}
				}
			}
			if typ != "" {
				found = true
				fmt.Fprintf(stdout, "%s%d:%d %s %s → %s\n", prefix, ln+1, i+1, string(url), typ, encode(body))
			}
			if j <= i {
				j = i + 1
			}
			i = j
		}
	}
	return found
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("urlsnag", flag.ContinueOnError)
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
		fmt.Fprintln(stderr, "Run 'urlsnag -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	files := fs.Args()
	found := false
	if len(files) == 0 {
		if f, ok := stdin.(*os.File); ok {
			if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'urlsnag -h' for usage.")
				return 2
			}
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintln(stderr, "urlsnag: "+err.Error())
			return 2
		}
		found = scan(string(data), "", stdout)
	} else {
		for _, name := range files {
			data, err := os.ReadFile(name)
			if err != nil {
				fmt.Fprintln(stderr, "urlsnag: "+err.Error())
				return 2
			}
			prefix := ""
			if len(files) > 1 {
				prefix = name + ":"
			}
			if scan(string(data), prefix, stdout) {
				found = true
			}
		}
	}
	if found {
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
