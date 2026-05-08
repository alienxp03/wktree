package tmux

import (
	"context"
	"strings"
	"testing"

	"github.com/alienxp03/wktree/internal/run"
	"github.com/alienxp03/wktree/internal/setup"
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
	command := VisibleCommand("pnpm install")
	for _, want := range []string{"$ pnpm install", "pnpm install", "post setup command failed"} {
		if !strings.Contains(command, want) {
			t.Fatalf("VisibleCommand missing %q: %s", want, command)
		}
	}
}

func TestOpenInsideTmux(t *testing.T) {
	runner := newTmuxRunner(t)
	status, err := Open(context.Background(), Options{
		WorktreePath: "/tmp/me/testing_name",
		RepoSlug:     "alienxp03_demo",
		BranchSlug:   "feature-example",
		SetupPlan: setup.Plan{
			RepoRoot:     "/tmp/source",
			WorktreePath: "/tmp/me/testing_name",
			PostSetup:    []string{"pnpm install"},
		},
		Env:    map[string]string{"TMUX": "/tmp/tmux"},
		Runner: run.RunnerFunc(runner.run),
		Logger: setup.Logger{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	got := firstArgs(runner.calls)
	want := []string{"-V", "has-session", "new-session", "send-keys", "switch-client"}
	assertSlice(t, got, want)
	if runner.calls[1].args[2] != "me/testing_name" {
		t.Fatalf("has-session args = %#v", runner.calls[1].args)
	}
	if runner.calls[2].args[6] != "me/testing_name" || runner.calls[2].args[8] != "/tmp/me/testing_name" || runner.calls[2].args[10] != "testing_name" {
		t.Fatalf("new-session args = %#v", runner.calls[2].args)
	}
	if runner.calls[3].args[2] != "%9" || !strings.Contains(runner.calls[3].args[3], "pnpm install") {
		t.Fatalf("send-keys args = %#v", runner.calls[3].args)
	}
	if runner.calls[4].args[2] != "me/testing_name" {
		t.Fatalf("switch-client args = %#v", runner.calls[4].args)
	}
}

func TestOpenOutsideTmux(t *testing.T) {
	runner := newTmuxRunner(t)
	status, err := Open(context.Background(), Options{
		WorktreePath: "/tmp/me/testing_name",
		RepoSlug:     "alienxp03_demo",
		BranchSlug:   "feature-example",
		SetupPlan: setup.Plan{
			RepoRoot:     "/tmp/source",
			WorktreePath: "/tmp/me/testing_name",
			PostSetup:    []string{"pnpm install"},
		},
		Env:    map[string]string{},
		Runner: run.RunnerFunc(runner.run),
		Logger: setup.Logger{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	got := firstArgs(runner.calls)
	want := []string{"-V", "has-session", "new-session", "send-keys", "attach-session"}
	assertSlice(t, got, want)
	if runner.calls[4].args[2] != "me/testing_name" {
		t.Fatalf("attach-session args = %#v", runner.calls[4].args)
	}
	if !runner.calls[4].options.Inherit {
		t.Fatal("expected attach to inherit stdio")
	}
}

func TestOpenFailsOnExistingSession(t *testing.T) {
	runner := newTmuxRunner(t)
	runner.hasSession = true
	_, err := Open(context.Background(), Options{
		WorktreePath: "/tmp/me/testing_name",
		RepoSlug:     "alienxp03_demo",
		BranchSlug:   "feature-example",
		SetupPlan:    setup.Plan{RepoRoot: "/tmp/source", WorktreePath: "/tmp/me/testing_name"},
		Env:          map[string]string{"TMUX": "/tmp/tmux"},
		Runner:       run.RunnerFunc(runner.run),
		Logger:       setup.Logger{},
	})
	if err == nil || !strings.Contains(err.Error(), "tmux session already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenFailsWhenNewSessionDoesNotReturnPane(t *testing.T) {
	runner := newTmuxRunner(t)
	runner.newSessionOutput = ""
	_, err := Open(context.Background(), Options{
		WorktreePath: "/tmp/me/testing_name",
		RepoSlug:     "alienxp03_demo",
		BranchSlug:   "feature-example",
		SetupPlan:    setup.Plan{RepoRoot: "/tmp/source", WorktreePath: "/tmp/me/testing_name"},
		Env:          map[string]string{"TMUX": "/tmp/tmux"},
		Runner:       run.RunnerFunc(runner.run),
		Logger:       setup.Logger{},
	})
	if err == nil || !strings.Contains(err.Error(), "tmux did not return a pane target") {
		t.Fatalf("err = %v", err)
	}
}

func TestKillSessionForWorktree(t *testing.T) {
	runner := newTmuxRunner(t)
	runner.hasSession = true

	err := KillSessionForWorktree(context.Background(), "/tmp/me/testing_name", run.RunnerFunc(runner.run))
	if err != nil {
		t.Fatal(err)
	}

	got := firstArgs(runner.calls)
	want := []string{"-V", "has-session", "kill-session"}
	assertSlice(t, got, want)
	if runner.calls[2].args[2] != "me/testing_name" {
		t.Fatalf("kill-session args = %#v", runner.calls[2].args)
	}
}

func TestKillSessionForWorktreeSkipsMissingSession(t *testing.T) {
	runner := newTmuxRunner(t)

	err := KillSessionForWorktree(context.Background(), "/tmp/me/testing_name", run.RunnerFunc(runner.run))
	if err != nil {
		t.Fatal(err)
	}

	got := firstArgs(runner.calls)
	want := []string{"-V", "has-session"}
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
}

func newTmuxRunner(t *testing.T) *tmuxRunner {
	return &tmuxRunner{t: t, newSessionOutput: "%9\n"}
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
	case "new-session":
		return run.Result{ExitCode: 0, Stdout: runner.newSessionOutput}
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
