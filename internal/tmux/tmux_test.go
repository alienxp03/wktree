package tmux

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alienxp03/wktree/internal/run"
)

func TestSessionName(t *testing.T) {
	if got := SessionName("/tmp/me/testing_name"); got != "me/testing_name" {
		t.Fatalf("SessionName = %q", got)
	}
	if got := SessionName("/tmp/worktrees/testing__feature-update-readme-1"); got != "worktrees/testing__feature-update-readme-1" {
		t.Fatalf("SessionName = %q", got)
	}
	if got := SessionName("///"); got != "wktree" {
		t.Fatalf("fallback SessionName = %q", got)
	}
}

func TestWindowName(t *testing.T) {
	if got := WindowName("/tmp/me/testing_name"); got != "testing_name" {
		t.Fatalf("WindowName = %q", got)
	}
	if got := WindowName("///"); got != "wktree" {
		t.Fatalf("fallback WindowName = %q", got)
	}
}

func TestVisibleCommand(t *testing.T) {
	command := VisibleCommand("/tmp/worktree", "pnpm install")
	for _, want := range []string{"$ pnpm install", ". '/tmp/worktree/.wktree.env'", "eval 'pnpm install'", "wktree_status=$?", "pane command failed", "exec \"${SHELL:-/bin/sh}\" -i"} {
		if !strings.Contains(command, want) {
			t.Fatalf("VisibleCommand missing %q: %s", want, command)
		}
	}
}

func TestPaneShellCommandUsesNonInteractiveCommandShell(t *testing.T) {
	command := PaneShellCommand("/tmp/worktree", "pnpm install")
	for _, want := range []string{"exec \"${SHELL:-/bin/sh}\" -fc", "$ pnpm install", "/tmp/worktree/.wktree.env", "eval", "pnpm install", ". \"${ZDOTDIR:-$HOME}/.zshrc\""} {
		if !strings.Contains(command, want) {
			t.Fatalf("PaneShellCommand missing %q: %s", want, command)
		}
	}
}

func TestPaneShellCommandSupportsAliases(t *testing.T) {
	shell, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not found")
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	worktreePath := filepath.Join(root, "worktree")
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, ".wktree.env"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("echo startup-noise >&2\nalias cox='echo alias-ok'\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("/bin/sh", "-c", PaneShellCommand(worktreePath, "cox; exit"))
	cmd.Env = append(os.Environ(), "HOME="+home, "SHELL="+shell)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "alias-ok") {
		t.Fatalf("alias did not run:\n%s", output)
	}
	if strings.Contains(string(output), "startup-noise") {
		t.Fatalf("startup output was not suppressed:\n%s", output)
	}
}

func TestOpenWindowLayout(t *testing.T) {
	runner := newTmuxRunner(t)
	status, err := OpenLayout(context.Background(), LayoutOptions{
		Mode: ModeWindow,
		Windows: []Window{{
			Name:         "feature_backend",
			WorktreePath: "/tmp/backend",
			Commands: []PaneCommand{
				{Command: "nvim", Focus: true},
				{Commands: []string{"pnpm install", "pnpm run dev"}, Split: "vertical", Percentage: 40},
			},
		}},
		Env:    map[string]string{"TMUX": "/tmp/tmux"},
		Runner: run.RunnerFunc(runner.run),
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	got := firstArgs(runner.calls)
	want := []string{"-V", "list-windows", "new-window", "respawn-pane", "split-window", "respawn-pane", "select-pane", "select-window"}
	assertSlice(t, got, want)
	if runner.calls[2].args[5] != "/tmp/backend" || runner.calls[2].args[7] != "feature_backend" {
		t.Fatalf("new-window args = %#v", runner.calls[2].args)
	}
	if !containsArg(runner.calls[4].args, "-v") || !containsArg(runner.calls[4].args, "-p") {
		t.Fatalf("split-window args = %#v", runner.calls[4].args)
	}
	if !strings.Contains(lastArg(runner.calls[5].args), "pnpm install && pnpm run dev") {
		t.Fatalf("respawn-pane args = %#v", runner.calls[5].args)
	}
}

func TestOpenWindowLayoutSplitsFromPreviousPane(t *testing.T) {
	runner := newTmuxRunner(t)
	status, err := OpenLayout(context.Background(), LayoutOptions{
		Mode: ModeWindow,
		Windows: []Window{{
			Name:         "feature_backend",
			WorktreePath: "/tmp/backend",
			Commands: []PaneCommand{
				{Command: "nvim", Focus: true},
				{Commands: []string{`echo "test"`}, Split: "horizontal"},
				{Commands: []string{"cox"}, Split: "vertical"},
			},
		}},
		Env:    map[string]string{"TMUX": "/tmp/tmux"},
		Runner: run.RunnerFunc(runner.run),
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if targetArg(runner.calls[4].args) != "%1" {
		t.Fatalf("second pane split target = %#v", runner.calls[4].args)
	}
	if targetArg(runner.calls[6].args) != "%2" {
		t.Fatalf("third pane split target = %#v", runner.calls[6].args)
	}
	if !containsArg(runner.calls[4].args, "-h") || !containsArg(runner.calls[6].args, "-v") {
		t.Fatalf("split args = %#v %#v", runner.calls[4].args, runner.calls[6].args)
	}
}

func TestOpenWindowLayoutRequiresTmux(t *testing.T) {
	runner := newTmuxRunner(t)
	_, err := OpenLayout(context.Background(), LayoutOptions{
		Mode:    ModeWindow,
		Windows: []Window{{Name: "feature_backend", WorktreePath: "/tmp/backend"}},
		Env:     map[string]string{},
		Runner:  run.RunnerFunc(runner.run),
	})
	if err == nil || !strings.Contains(err.Error(), "requires running inside tmux") {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenSessionLayout(t *testing.T) {
	runner := newTmuxRunner(t)
	status, err := OpenLayout(context.Background(), LayoutOptions{
		Mode:        ModeSession,
		SessionName: "repo/feature",
		Windows: []Window{
			{Name: "backend", WorktreePath: "/tmp/backend", Commands: []PaneCommand{{Command: "nvim"}}},
			{Name: "frontend", WorktreePath: "/tmp/frontend"},
		},
		Env:    map[string]string{"TMUX": "/tmp/tmux"},
		Runner: run.RunnerFunc(runner.run),
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	got := firstArgs(runner.calls)
	want := []string{"-V", "has-session", "new-session", "respawn-pane", "select-pane", "new-window", "select-pane", "switch-client"}
	assertSlice(t, got, want)
	if runner.calls[2].args[6] != "repo/feature" || runner.calls[5].args[5] != "repo/feature:" {
		t.Fatalf("session args = %#v %#v", runner.calls[2].args, runner.calls[5].args)
	}
}

func TestOpenSessionSwitchesExistingSession(t *testing.T) {
	runner := newTmuxRunner(t)
	runner.hasSession = true
	status, err := OpenLayout(context.Background(), LayoutOptions{
		Mode:        ModeSession,
		SessionName: "repo/feature",
		Windows:     []Window{{Name: "backend", WorktreePath: "/tmp/backend"}},
		Env:         map[string]string{"TMUX": "/tmp/tmux"},
		Runner:      run.RunnerFunc(runner.run),
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	got := firstArgs(runner.calls)
	want := []string{"-V", "has-session", "switch-client"}
	assertSlice(t, got, want)
}

func TestKillLayout(t *testing.T) {
	runner := newTmuxRunner(t)
	runner.hasSession = true

	err := KillLayout(context.Background(), KillOptions{
		Mode:        ModeSession,
		SessionName: "repo/feature",
		KillSession: true,
		Runner:      run.RunnerFunc(runner.run),
	})
	if err != nil {
		t.Fatal(err)
	}

	got := firstArgs(runner.calls)
	want := []string{"-V", "has-session", "kill-session"}
	assertSlice(t, got, want)
}

type tmuxCall struct {
	command string
	args    []string
	options run.Options
}

type tmuxRunner struct {
	t                *testing.T
	calls            []tmuxCall
	hasSession       bool
	newSessionOutput string
	nextPane         int
}

func newTmuxRunner(t *testing.T) *tmuxRunner {
	return &tmuxRunner{t: t, newSessionOutput: "%9\n", nextPane: 10}
}

func (runner *tmuxRunner) run(ctx context.Context, command string, args []string, options run.Options) run.Result {
	runner.calls = append(runner.calls, tmuxCall{command: command, args: append([]string(nil), args...), options: options})
	if command != "tmux" {
		runner.t.Fatalf("command = %s", command)
	}
	switch args[0] {
	case "-V":
		return run.Result{ExitCode: 0, Stdout: "tmux 3.5\n"}
	case "has-session":
		if runner.hasSession {
			return run.Result{ExitCode: 0}
		}
		return run.Result{ExitCode: 1}
	case "list-windows":
		return run.Result{ExitCode: 0}
	case "new-session":
		return run.Result{ExitCode: 0, Stdout: runner.newSessionOutput}
	case "new-window", "split-window":
		runner.nextPane++
		return run.Result{ExitCode: 0, Stdout: "%" + string(rune('0'+runner.nextPane%10)) + "\n"}
	default:
		return run.Result{ExitCode: 0}
	}
}

func firstArgs(calls []tmuxCall) []string {
	values := make([]string, 0, len(calls))
	for _, call := range calls {
		values = append(values, call.args[0])
	}
	return values
}

func containsArg(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func targetArg(values []string) string {
	for index, value := range values {
		if value == "-t" && index+1 < len(values) {
			return values[index+1]
		}
	}
	return ""
}

func lastArg(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func assertSlice(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}
