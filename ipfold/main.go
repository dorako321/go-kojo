// ipfold: ipfold extracts unique IPv4 addresses from text and prints them grouped by prefix, wrapped to a given width.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `ipfold extracts unique IPv4 addresses from text and prints them grouped by prefix, wrapped to a given width.

Usage:
  ipfold [-p 8|16|24] [-w width] [file ...]

Examples:
  echo "10.0.1.5 10.0.1.33 10.0.2.7" | ipfold
  ipfold -p 16 -w 60 access.log
  ipfold -p 8 a.txt b.txt

Flags:
`

var ipRe = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+`)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ipfold", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	showVersion := fs.Bool("version", false, "print version and exit")
	prefix := fs.Int("p", 24, "prefix length: 8, 16 or 24")
	width := fs.Int("w", 80, "display width for wrapping (must be > 0)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			fs.SetOutput(stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintln(stderr, "Run 'ipfold -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}
	if *prefix != 8 && *prefix != 16 && *prefix != 24 {
		fmt.Fprintln(stderr, "ipfold: -p must be 8, 16 or 24")
		return 2
	}
	if *width < 1 {
		fmt.Fprintln(stderr, "ipfold: -w must be greater than 0")
		return 2
	}

	code := 0
	var data []byte
	if fs.NArg() == 0 {
		if f, ok := stdin.(*os.File); ok {
			if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(stderr, "Run 'ipfold -h' for usage.")
				return 2
			}
		}
		b, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "ipfold: %v\n", err)
			code = 1
		}
		data = b
	} else {
		for _, name := range fs.Args() {
			b, err := os.ReadFile(name)
			if err != nil {
				fmt.Fprintf(stderr, "ipfold: %v\n", err)
				code = 1
				continue
			}
			data = append(data, b...)
		}
	}

	seen := map[[4]int]bool{}
	var addrs [][4]int
	for _, m := range ipRe.FindAll(data, -1) {
		parts := strings.Split(string(m), ".")
		var a [4]int
		ok := true
		for i, p := range parts {
			if len(p) > 3 {
				ok = false
				break
			}
			n, err := strconv.Atoi(p)
			if err != nil || n > 255 {
				ok = false
				break
			}
			a[i] = n
		}
		if ok && !seen[a] {
			seen[a] = true
			addrs = append(addrs, a)
		}
	}
	sort.Slice(addrs, func(i, j int) bool {
		for k := 0; k < 4; k++ {
			if addrs[i][k] != addrs[j][k] {
				return addrs[i][k] < addrs[j][k]
			}
		}
		return false
	})

	np := *prefix / 8
	var sb strings.Builder
	lastHead := ""
	lineLen := 0
	for _, a := range addrs {
		var h strings.Builder
		for i := 0; i < np; i++ {
			h.WriteString(strconv.Itoa(a[i]))
			h.WriteString(".")
		}
		head := h.String() + ":"
		rest := make([]string, 0, 4)
		for i := np; i < 4; i++ {
			rest = append(rest, strconv.Itoa(a[i]))
		}
		val := strings.Join(rest, ".")
		if head != lastHead {
			if lastHead != "" {
				sb.WriteString("\n")
			}
			lastHead = head
			sb.WriteString(head + " " + val)
			lineLen = len(head) + 1 + len(val)
			continue
		}
		if lineLen+1+len(val) > *width {
			pad := strings.Repeat(" ", len(head)+1)
			sb.WriteString("\n" + pad + val)
			lineLen = len(pad) + len(val)
		} else {
			sb.WriteString(" " + val)
			lineLen += 1 + len(val)
		}
	}
	if lastHead != "" {
		sb.WriteString("\n")
	}
	fmt.Fprint(stdout, sb.String())
	return code
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
