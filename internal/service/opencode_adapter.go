package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/stefenello/agent-orchestrator/internal/domain"
)

// CLIAdapter executes OpenCode phases by invoking the local `opencode` binary.
// It only uses flags confirmed by Available; unverified capabilities are
// treated as absent. Piave swaps only the process launcher, not this contract.
type CLIAdapter struct {
	binary  string
	timeout time.Duration
	caps    domain.CLICapabilities
	probed  bool
}

func NewCLIAdapter(binary string) *CLIAdapter {
	if binary == "" {
		binary = "opencode"
	}
	return &CLIAdapter{binary: binary, timeout: 10 * time.Minute}
}

var _ domain.Runtime = (*CLIAdapter)(nil)

// Name identifies the runtime.
func (a *CLIAdapter) Name() string { return "opencode" }

// Capabilities returns the last probed capabilities (zero value if never probed).
func (a *CLIAdapter) Capabilities() domain.CLICapabilities { return a.caps }

// Available probes the installed CLI. It requires only that `--version`
// works; everything else is discovered and marked absent when unavailable.
// A failure wraps ErrRuntimeUnavailable.
func (a *CLIAdapter) Available(ctx context.Context) (domain.CLICapabilities, error) {
	version, verErr := a.output(ctx, "--version")
	runHelp, _ := a.output(ctx, "run", "--help")
	agents, _ := a.output(ctx, "agent", "list")

	caps := ParseCapabilities(version, runHelp, agents)
	if verErr != nil {
		return caps, fmt.Errorf("%w: %v", domain.ErrRuntimeUnavailable, verErr)
	}
	a.caps = caps
	a.probed = true
	return caps, nil
}

// Run executes one phase. A non-zero process exit is returned as evidence in
// RunResult.ExitCode, not as an error; an error means the process could not be
// executed at all.
func (a *CLIAdapter) Run(ctx context.Context, req domain.RunRequest) (*domain.RunResult, error) {
	if req.WorktreePath == "" {
		return nil, errors.New("worktree path is required")
	}
	if !a.probed {
		if _, err := a.Available(ctx); err != nil {
			return nil, err
		}
	}

	preHead, _ := revParse(ctx, req.WorktreePath)

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = a.timeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, a.binary, a.buildArgs(req)...)
	cmd.Dir = req.WorktreePath
	cmd.Env = append(os.Environ(), mapToEnv(req.EnvVars)...)
	// Kill the whole process group on timeout/cancellation so no child process
	// (e.g. an opencode subprocess) is left orphaned.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second

	if os.Getenv("ORCHESTRATOR_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "opencode exec: %s %q (dir=%s)\n", a.binary, cmd.Args[1:], cmd.Dir)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exitCode := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			// The process could not be started (e.g. binary missing/not executable).
			return nil, fmt.Errorf("%w: %v", domain.ErrProcessStart, err)
		}
	}

	postHead, _ := revParse(ctx, req.WorktreePath)

	base := req.BaseCommitSHA
	if base == "" {
		base = preHead
	}

	res := &domain.RunResult{
		ExitCode:      exitCode,
		Stdout:        stdout.String(),
		Stderr:        stderr.String(),
		AgentText:     extractAgentText(stdout.String()),
		TimedOut:      errors.Is(runCtx.Err(), context.DeadlineExceeded),
		Canceled:      errors.Is(runCtx.Err(), context.Canceled),
		BaseCommitSHA: base,
		CommitSHA:     postHead,
	}
	if base != "" && postHead != "" && base != postHead {
		if diff, err := runGitCommand(ctx, req.WorktreePath, "diff", base+".."+postHead); err == nil {
			res.Diff = diff
		}
		if stat, err := runGitCommand(ctx, req.WorktreePath, "diff", "--stat", base+".."+postHead); err == nil {
			res.DiffStat = stat
		}
	}
	return res, nil
}

func (a *CLIAdapter) buildArgs(req domain.RunRequest) []string {
	args := []string{"run"}
	if a.caps.SupportsDir {
		args = append(args, "--dir", req.WorktreePath)
	}
	if a.caps.SupportsJSON {
		args = append(args, "--format", "json")
	}
	if a.caps.SupportsAgent && req.Agent != "" {
		args = append(args, "--agent", req.Agent)
	}
	if a.caps.SupportsModel && req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	// Session continuity is best-effort and never load-bearing.
	if a.caps.SupportsSession && req.SessionID != "" {
		args = append(args, "--session", req.SessionID)
		if req.Fork && a.caps.SupportsFork {
			args = append(args, "--fork")
		}
	}
	return append(args, req.Prompt)
}

// output runs a probe command and returns combined stdout+stderr. OpenCode
// writes `run --help` to stderr, so both streams must be captured.
func (a *CLIAdapter) output(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, a.binary, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func revParse(ctx context.Context, dir string) (string, error) {
	out, err := runGitCommand(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func mapToEnv(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// extractAgentText pulls assistant text parts out of OpenCode's `--format json`
// JSONL stream. It is deliberately tolerant: unrecognized lines are ignored.
func extractAgentText(stdout string) string {
	var b strings.Builder
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var ev struct {
			Type string `json:"type"`
			Part struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"part"`
		}
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Type == "text" && ev.Part.Type == "text" && ev.Part.Text != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(ev.Part.Text)
		}
	}
	return strings.TrimSpace(b.String())
}
