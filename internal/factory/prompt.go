package factory

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dorako321/go-kojo/internal/inspect"
	"github.com/dorako321/go-kojo/internal/spec"
)

// houseRules is the same for every product: it is what makes the output of
// the factory look like it came from one hand.
const houseRules = `You write one small Go command-line program per request for a factory that mass-produces single-purpose CLIs. You are given a spec, a frame of main.go, and sometimes documentation of the allowed dependencies. You reply through the JSON schema only.

House rules for main.go:
- One file, package main, at most 200 lines, formatted as gofmt would.
- Start from the frame and keep these parts exactly as they behave now: func main, the signature "func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int", the version variable, buildVersion, and the handling of -h, -version and flag parse errors. Add the spec's flags to the same FlagSet. Replace the TODO under "Examples:" in the usage text with two or three real invocations.
- Inside run, read input only from the stdin parameter and write only to the stdout and stderr parameters. Never call os.Exit outside main.
- Parse arguments with the standard flag package. No subcommands. Add no flags or features the spec does not list.
- Import only the standard library and the dependencies declared in the spec. Never import os/exec, syscall or unsafe. Use net packages only when the spec says network is true.
- Results go to stdout. Errors go to stderr as "<name>: <message>" and return 1. Usage errors return 2. Success returns 0.
- Never prompt and never wait for a terminal. If the program would read stdin and stdin is a terminal (an *os.File whose mode has os.ModeCharDevice), print "Run '<name> -h' for usage." to stderr and return 2.
- Do not declare identifiers that start with "kojo"; the generated test file uses that prefix.
- Implement the numbered behaviors exactly. The acceptance cases are executed against your program, and further cases you cannot see may be too, so never recognise particular inputs.

Reply:
- status "ok" with the complete main.go in main_go.
- status "cannot" with the reason when the spec cannot be met inside the house rules. Leave main_go empty.
- status "dispute" with the reason when an acceptance case contradicts the numbered behaviors. Leave main_go empty.`

func buildPrompt(sp spec.Spec, frame, docs string) string {
	js, _ := json.MarshalIndent(sp, "", "  ")
	var b strings.Builder
	fmt.Fprintf(&b, "## Spec\n\n```json\n%s\n```\n\n## Frame of main.go\n\n```go\n%s```\n", js, frame)
	if docs != "" {
		fmt.Fprintf(&b, "\n## Documentation of the declared dependencies\n%s", docs)
	}
	b.WriteString("\nWrite the complete main.go.\n")
	return b.String()
}

func repairPrompt(sp spec.Spec, frame, docs, source string, failed []inspect.Gate) string {
	var b strings.Builder
	b.WriteString(buildPrompt(sp, frame, docs))
	fmt.Fprintf(&b, "\n## Your previous main.go\n\n```go\n%s```\n\n## It did not pass inspection\n", source)
	for _, g := range failed {
		fmt.Fprintf(&b, "\n### %s\n\n%s\n", g.Name, g.Detail)
		if g.Hint != "" {
			fmt.Fprintf(&b, "\nWhat to do: %s\n", g.Hint)
		}
	}
	b.WriteString("\nReturn the corrected, complete main.go.\n")
	return b.String()
}
