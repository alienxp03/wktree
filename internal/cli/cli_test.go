package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alienxp03/wktree/internal/config"
	"github.com/alienxp03/wktree/internal/tmux"
)

func TestHelpAndVersion(t *testing.T) {
	stdout := &bytes.Buffer{}
	status := Run([]string{"--help"}, Options{Stdout: stdout, Stderr: &bytes.Buffer{}})
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stdout.String(), "wktree - create strict Git worktrees") {
		t.Fatalf("help = %q", stdout.String())
	}

	stdout.Reset()
	status = Run([]string{"--version"}, Options{Stdout: stdout, Stderr: &bytes.Buffer{}})
	if status != 0 || strings.TrimSpace(stdout.String()) != Version {
		t.Fatalf("version status=%d stdout=%q", status, stdout.String())
	}
}

func TestCompletion(t *testing.T) {
	stdout := &bytes.Buffer{}
	status := Run([]string{"completion", "bash"}, Options{Stdout: stdout, Stderr: &bytes.Buffer{}})
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stdout.String(), "complete -F _wktree_completion wktree") {
		t.Fatalf("completion = %q", stdout.String())
	}

	stdout.Reset()
	status = Run([]string{"init", "bash"}, Options{Stdout: stdout, Stderr: &bytes.Buffer{}})
	if status != 0 {
		t.Fatalf("legacy status = %d", status)
	}
	if !strings.Contains(stdout.String(), "complete -F _wktree_completion wktree") {
		t.Fatalf("legacy init = %q", stdout.String())
	}
}

func TestInvalidNewUsage(t *testing.T) {
	stderr := &bytes.Buffer{}
	status := Run([]string{"new"}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "usage: wktree new") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestInvalidListUsage(t *testing.T) {
	stderr := &bytes.Buffer{}
	status := Run([]string{"list", "extra"}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "usage: wktree list") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestInvalidDoctorUsage(t *testing.T) {
	stderr := &bytes.Buffer{}
	status := Run([]string{"doctor", "extra"}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "usage: wktree doctor") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestInvalidRemoveUsage(t *testing.T) {
	stderr := &bytes.Buffer{}
	status := Run([]string{"remove"}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "usage: wktree remove") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestInvalidSwitchUsage(t *testing.T) {
	stderr := &bytes.Buffer{}
	status := Run([]string{"switch"}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "usage: wktree switch") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestWorkspaceWindowNameUsesWorkspaceName(t *testing.T) {
	if got := workspaceWindowName("window_1"); got != "window_1" {
		t.Fatalf("workspaceWindowName = %q", got)
	}
	if got := workspaceWindowName("window 1:api"); got != "window-1-api" {
		t.Fatalf("sanitized workspaceWindowName = %q", got)
	}
}

func TestEffectiveTmuxModeForcesSessionForAllWorkspaces(t *testing.T) {
	selection := workspaceSelection{
		Config:        config.Config{TmuxMode: tmux.ModeWindow},
		AllWorkspaces: true,
	}
	if got := effectiveTmuxMode(selection); got != tmux.ModeSession {
		t.Fatalf("effective tmux mode = %q", got)
	}

	selection.AllWorkspaces = false
	if got := effectiveTmuxMode(selection); got != tmux.ModeWindow {
		t.Fatalf("single workspace effective tmux mode = %q", got)
	}
}
