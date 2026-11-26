// Package engine is the factory's only contact with a language model:
// a prompt and a JSON Schema go in, one JSON value comes out.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Request struct {
	System string
	Prompt string
	Schema string // JSON Schema the output must satisfy
	Model  string
}

type Response struct {
	Output   json.RawMessage // the structured output
	CostUSD  float64         // list-price estimate reported by the engine
	Duration time.Duration
	Raw      []byte // the engine's full reply, for the run log
}

type Engine interface {
	Call(ctx context.Context, req Request) (Response, error)
}

// Claude drives the Claude Code CLI in print mode with every tool removed,
// so the model can only answer; it cannot read or change anything.
type Claude struct {
	Bin     string
	Timeout time.Duration
}

func NewClaude() (*Claude, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return nil, errors.New("claude CLI not found in PATH")
	}
	return &Claude{Bin: bin, Timeout: 10 * time.Minute}, nil
}

type claudeResult struct {
	IsError          bool            `json:"is_error"`
	Subtype          string          `json:"subtype"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	TotalCostUSD     float64         `json:"total_cost_usd"`
}

// Call asks once and, if the CLI fails for a reason other than the caller
// giving up, asks once more: a line that runs unattended should not lose a
// product to one dropped connection.
func (c *Claude) Call(ctx context.Context, req Request) (Response, error) {
	resp, err := c.call(ctx, req)
	if err == nil || ctx.Err() != nil {
		return resp, err
	}
	select {
	case <-time.After(3 * time.Second):
	case <-ctx.Done():
		return resp, err
	}
	again, err2 := c.call(ctx, req)
	again.CostUSD += resp.CostUSD
	again.Duration += resp.Duration
	if err2 != nil {
		return again, fmt.Errorf("%w (and again: %v)", err, err2)
	}
	return again, nil
}

func (c *Claude) call(ctx context.Context, req Request) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	// An empty directory keeps project files and settings out of the call.
	work, err := os.MkdirTemp("", "kojo-engine-")
	if err != nil {
		return Response{}, err
	}
	defer os.RemoveAll(work)

	cmd := exec.CommandContext(ctx, c.Bin,
		"-p", "--safe-mode", "--tools", "",
		"--output-format", "json", "--no-session-persistence",
		"--model", req.Model,
		"--system-prompt", req.System,
		"--json-schema", req.Schema,
	)
	cmd.Dir = work
	cmd.Env = allowedEnv()
	cmd.Stdin = strings.NewReader(req.Prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second

	start := time.Now()
	runErr := cmd.Run()
	resp := Response{Duration: time.Since(start), Raw: stdout.Bytes()}

	var res claudeResult
	parseErr := json.Unmarshal(stdout.Bytes(), &res)
	resp.CostUSD = res.TotalCostUSD

	switch {
	case ctx.Err() != nil:
		return resp, fmt.Errorf("claude: %w after %s", ctx.Err(), resp.Duration.Round(time.Second))
	case runErr != nil:
		// The CLI reports most failures inside its JSON reply, not on stderr.
		why := stderr.String() + res.Result
		if strings.TrimSpace(why) == "" {
			why = res.Subtype + " " + stdout.String()
		}
		return resp, fmt.Errorf("claude: %v: %s", runErr, tail(why))
	case parseErr != nil:
		return resp, fmt.Errorf("claude: unreadable reply: %v", parseErr)
	case res.IsError:
		return resp, fmt.Errorf("claude: %s: %s", res.Subtype, tail(res.Result))
	case len(res.StructuredOutput) == 0 || string(res.StructuredOutput) == "null":
		return resp, fmt.Errorf("claude: no structured output: %s", tail(res.Result))
	}
	resp.Output = res.StructuredOutput
	return resp, nil
}

// allowedEnv passes only what the CLI needs to find its login. Variables of
// an enclosing Claude Code session and API keys are deliberately dropped.
func allowedEnv() []string {
	var env []string
	for _, k := range []string{"HOME", "PATH", "USER", "LOGNAME", "LANG", "LC_ALL", "TMPDIR", "SHELL"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		return "…" + s[len(s)-400:]
	}
	return s
}
