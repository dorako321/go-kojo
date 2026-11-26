// Package ship moves a finished product out of the factory: onto the PATH
// of this machine, or to a public GitHub repository. Nothing here is called
// until the user has explicitly chosen it.
package ship

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Install copies the built program into a directory on PATH and returns
// where it went.
func Install(binary, name string) (string, error) {
	dir, err := binDir()
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, name)
	if err := copyFile(binary, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// binDir prefers ~/.local/bin, then Go's own bin directory, and only uses
// one that is already on PATH.
func binDir() (string, error) {
	home, _ := os.UserHomeDir()
	candidates := []string{filepath.Join(home, ".local", "bin")}
	if out, err := exec.Command("go", "env", "GOBIN", "GOPATH").Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 0 && lines[0] != "" {
			candidates = append(candidates, lines[0])
		}
		if len(lines) > 1 && lines[1] != "" {
			candidates = append(candidates, filepath.Join(lines[1], "bin"))
		}
	}
	onPath := map[string]bool{}
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		onPath[filepath.Clean(p)] = true
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() && onPath[filepath.Clean(c)] {
			return c, nil
		}
	}
	return "", fmt.Errorf("no writable bin directory on PATH (tried %s)", strings.Join(candidates, ", "))
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// CopyBinary places the built program inside the product directory.
func CopyBinary(binary, dir, name string) (string, error) {
	dst := filepath.Join(dir, name)
	return dst, copyFile(binary, dst)
}

// Plan is what Publish will do, for showing to the user beforehand.
type Plan struct {
	Repo     string   // owner/name
	Files    []string // files that will become public
	Commands []string
}

// Prepare checks that publishing can go ahead and describes it. It changes
// nothing.
func Prepare(ctx context.Context, dir, owner, name string) (Plan, error) {
	plan := Plan{Repo: owner + "/" + name}
	if err := exec.CommandContext(ctx, "gh", "auth", "status").Run(); err != nil {
		return plan, fmt.Errorf("gh is not logged in; run: gh auth login")
	}
	if exec.CommandContext(ctx, "gh", "repo", "view", plan.Repo).Run() == nil {
		return plan, fmt.Errorf("the repository %s already exists", plan.Repo)
	}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if rel != name { // the binary itself is ignored by .gitignore
			plan.Files = append(plan.Files, rel)
		}
		return nil
	})
	plan.Commands = []string{
		"git init -b main && git add -A && git commit",
		"gh repo create " + plan.Repo + " --public --source=. --remote=origin --push",
	}
	return plan, err
}

// Publish commits the product and pushes it to a new public repository.
// It returns the repository URL.
func Publish(ctx context.Context, dir, owner, name, summary string) (string, error) {
	repo := owner + "/" + name
	steps := [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "Initial commit\n\n" + summary + "\n\nBuilt by go-kojo."},
		{"gh", "repo", "create", repo, "--public", "--source=.", "--remote=origin", "--push", "--description", summary},
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		steps = append([][]string{{"git", "init", "-b", "main"}}, steps...)
	}
	for _, s := range steps {
		cmd := exec.CommandContext(ctx, s[0], s[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("%s %s: %s", s[0], s[1], strings.TrimSpace(string(out)))
		}
	}
	return "https://github.com/" + repo, nil
}
