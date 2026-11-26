package factory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dorako321/go-kojo/internal/engine"
	"github.com/dorako321/go-kojo/internal/mold"
	"github.com/dorako321/go-kojo/internal/spec"
)

// scripted is an engine that hands back prepared programs, one per call,
// and remembers what it was asked.
type scripted struct {
	sources []string
	prompts []string
}

func (s *scripted) Call(_ context.Context, req engine.Request) (engine.Response, error) {
	s.prompts = append(s.prompts, req.Prompt)
	src := s.sources[min(len(s.prompts), len(s.sources))-1]
	out, _ := json.Marshal(map[string]string{"status": "ok", "reason": "", "main_go": src})
	return engine.Response{Output: out, CostUSD: 0.01}, nil
}

func upperSpec() spec.Spec {
	want := "HELLO\n"
	return spec.Spec{
		Name: "upper", Summary: "upper prints its input in upper case.", Kind: "convert", Usage: "upper",
		Behavior: []string{"B1: Copy stdin to stdout in upper case.", "B2: Empty input prints nothing."},
		Cases: []spec.Case{
			{Name: "hello", Stdin: "hello\n", Exit: 0, Stdout: spec.Expect{Exact: &want}},
			{Name: "empty", Stdin: "", Exit: 0, Stdout: spec.Expect{Exact: new(string)}},
		},
	}
}

// program fills the frame's placeholder with a body, the way the engine is
// asked to.
func program(t *testing.T, sp spec.Spec, imports, body string) string {
	t.Helper()
	frame, err := mold.Frame(mold.Data{Spec: sp})
	if err != nil {
		t.Fatal(err)
	}
	src := strings.Replace(string(frame), "\t\"io\"\n", "\t\"io\"\n"+imports, 1)
	stub := "\tfmt.Fprintln(stderr, \"upper: not implemented\")\n\treturn 1\n"
	if !strings.Contains(src, stub) {
		t.Fatal("the frame no longer contains the stub this test replaces")
	}
	return strings.Replace(src, stub, body, 1)
}

const upperBody = "\tb, _ := io.ReadAll(stdin)\n\tfmt.Fprint(stdout, strings.ToUpper(string(b)))\n\treturn 0\n"

func build(t *testing.T, eng engine.Engine, sp spec.Spec) Report {
	t.Helper()
	if testing.Short() {
		t.Skip("compiles real programs")
	}
	dir := t.TempDir()
	rep, err := Build(context.Background(), sp, filepath.Join(dir, sp.Name), Options{
		Engine: eng, Model: "test", Rescue: "strong", Owner: "example", Holder: "tester", MaxRepairs: 2,
		RunDir: filepath.Join(dir, "run"),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return rep
}

func TestBuildPassesFirstTime(t *testing.T) {
	sp := upperSpec()
	eng := &scripted{sources: []string{program(t, sp, "\t\"strings\"\n", upperBody)}}
	rep := build(t, eng, sp)

	if rep.Status != "ok" || rep.Repairs() != 0 {
		t.Fatalf("status %q after %d repairs: %s", rep.Status, rep.Repairs(), rep.Reason)
	}
	for _, f := range []string{"main.go", "main_test.go", "go.mod", "README.md", "LICENSE", ".gitignore", "kojo.json", ".github/workflows/ci.yml"} {
		if _, err := os.Stat(filepath.Join(rep.Dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	readme, _ := os.ReadFile(filepath.Join(rep.Dir, "README.md"))
	for _, want := range []string{"go install github.com/example/upper@latest", "$ echo hello | upper", "HELLO", "Usage:"} {
		if !strings.Contains(string(readme), want) {
			t.Errorf("README lacks %q:\n%s", want, readme)
		}
	}
}

func TestBuildRepairsAfterFailedInspection(t *testing.T) {
	sp := upperSpec()
	wrong := program(t, sp, "", "\tio.Copy(stdout, stdin)\n\treturn 0\n") // forgets to upper-case
	right := program(t, sp, "\t\"strings\"\n", upperBody)
	eng := &scripted{sources: []string{wrong, right}}
	rep := build(t, eng, sp)

	if rep.Status != "ok" || rep.Repairs() != 1 {
		t.Fatalf("status %q after %d repairs: %s", rep.Status, rep.Repairs(), rep.Reason)
	}
	// The repair request must carry what went wrong, not just "try again".
	for _, want := range []string{`case "hello"`, `want exactly "HELLO\n"`, `got "hello\n"`, "io.Copy(stdout, stdin)"} {
		if !strings.Contains(eng.prompts[1], want) {
			t.Errorf("repair prompt lacks %q", want)
		}
	}
}

func TestBuildGivesUpAfterRepairs(t *testing.T) {
	sp := upperSpec()
	eng := &scripted{sources: []string{program(t, sp, "", "\tio.Copy(stdout, stdin)\n\treturn 0\n")}}
	rep := build(t, eng, sp)

	if rep.Status != "failed" || len(rep.Attempts) != 3 {
		t.Fatalf("status %q after %d attempts", rep.Status, len(rep.Attempts))
	}
	if got := rep.FailedGates(); len(got) != 1 || got[0] != "cases" {
		t.Errorf("failed gates %q", got)
	}
	if !rep.Created {
		t.Error("the report does not say the directory is the factory's to remove")
	}
	// The first two attempts are the builder's; only the last goes to rescue.
	var models []string
	for _, a := range rep.Attempts {
		models = append(models, a.Model)
	}
	if got := strings.Join(models, " "); got != "test test strong" {
		t.Errorf("attempts were made by %q", got)
	}
}

// An undeclared import must be refused before anything is downloaded.
func TestBuildRefusesUndeclaredImport(t *testing.T) {
	sp := upperSpec()
	src := program(t, sp, "\t_ \"github.com/nobody/nothing\"\n\t\"os/exec\"\n", "\t_ = exec.Command\n\treturn 0\n")
	rep := build(t, &scripted{sources: []string{src}}, sp)

	if rep.Status != "failed" {
		t.Fatalf("status %q", rep.Status)
	}
	gates := rep.Attempts[0].Gates
	if len(gates) != 1 || gates[0].Name != "imports" {
		t.Fatalf("inspection went past the imports gate: %+v", gates)
	}
	for _, want := range []string{"github.com/nobody/nothing", "os/exec"} {
		if !strings.Contains(gates[0].Detail, want) {
			t.Errorf("imports gate does not name %s: %s", want, gates[0].Detail)
		}
	}
}

func TestBuildRefusesExistingDirectory(t *testing.T) {
	dir := t.TempDir() // already exists
	rep, err := Build(context.Background(), upperSpec(), dir, Options{Engine: &scripted{}})
	if err == nil || rep.Created {
		t.Fatalf("Build accepted an existing directory (created=%v, err=%v)", rep.Created, err)
	}
}
