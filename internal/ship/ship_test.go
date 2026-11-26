package ship

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGH puts a stand-in for gh first on PATH. It records how it was called
// and never talks to GitHub: "repo view" says the repository does not exist,
// everything else succeeds.
func fakeGH(t *testing.T) (log string) {
	t.Helper()
	bin := t.TempDir()
	log = filepath.Join(bin, "gh.log")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\n[ \"$1 $2\" = \"repo view\" ] && exit 1\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for k, v := range map[string]string{
		"GIT_AUTHOR_NAME": "tester", "GIT_AUTHOR_EMAIL": "tester@example.com",
		"GIT_COMMITTER_NAME": "tester", "GIT_COMMITTER_EMAIL": "tester@example.com",
		"GIT_CONFIG_GLOBAL": filepath.Join(bin, "gitconfig"), "GIT_CONFIG_NOSYSTEM": "1",
	} {
		t.Setenv(k, v)
	}
	return log
}

func product(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tool")
	files := map[string]string{
		"main.go": "package main\nfunc main(){}\n", "go.mod": "module github.com/someone/tool\n",
		".gitignore": "/tool\n", "tool": "binary", ".github/workflows/ci.yml": "name: ci\n",
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPrepareListsWhatBecomesPublic(t *testing.T) {
	fakeGH(t)
	plan, err := Prepare(context.Background(), product(t), "someone", "tool")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(plan.Files, " ")
	if plan.Repo != "someone/tool" || strings.Contains(" "+got+" ", " tool ") || !strings.Contains(got, "main.go") {
		t.Errorf("repo %q, files %q", plan.Repo, got)
	}
}

func TestPublishCommitsAndCreatesPublicRepo(t *testing.T) {
	log := fakeGH(t)
	dir := product(t)
	url, err := Publish(context.Background(), dir, "someone", "tool", "tool does a thing.")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/someone/tool" {
		t.Errorf("url %q", url)
	}

	calls, _ := os.ReadFile(log)
	want := "repo create someone/tool --public --source=. --remote=origin --push --description tool does a thing."
	if !strings.Contains(string(calls), want) {
		t.Errorf("gh was called as:\n%s", calls)
	}

	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if n := strings.Count(git("log", "--oneline"), "\n"); n != 1 {
		t.Errorf("%d commits", n)
	}
	tracked := git("ls-files")
	if strings.Contains("\n"+tracked, "\ntool\n") || !strings.Contains(tracked, "main.go") {
		t.Errorf("tracked files:\n%s", tracked)
	}
	if branch := strings.TrimSpace(git("branch", "--show-current")); branch != "main" {
		t.Errorf("branch %q", branch)
	}
}
