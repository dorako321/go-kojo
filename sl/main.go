// sl: sl prints a small original ASCII art steam locomotive to standard output.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

// version is set at release time with -ldflags "-X main.version=v1.2.3".
var version = ""

const usage = `sl prints a small original ASCII art steam locomotive to standard output.

Usage:
  sl

Examples:
  sl
  sl -version
  sl -h

Flags:
`

const art = `     (  )
    (  )
  ___    _______
 |   |__|  _  _ |
 |__________________|
  (o)(o)(o)(o)(o)
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole program. It returns the exit code:
// 0 success, 1 failure, 2 usage error.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sl", flag.ContinueOnError)
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
		fmt.Fprintln(stderr, "Run 'sl -h' for usage.")
		return 2
	}
	if *showVersion {
		fmt.Fprintln(stdout, buildVersion())
		return 0
	}

	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "sl: unexpected argument: %s\n", fs.Arg(0))
		return 2
	}

	fmt.Fprint(stdout, art)
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
