package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alienxp03/wktree/internal/config"
	"github.com/alienxp03/wktree/internal/git"
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

	stdout.Reset()
	status = Run([]string{"__complete", "list", "--"}, Options{Stdout: stdout, Stderr: &bytes.Buffer{}})
	if status != 0 || !strings.Contains(stdout.String(), "--pr") {
		t.Fatalf("list completion status=%d stdout=%q", status, stdout.String())
	}
}

func TestSessionNameUsesTemplate(t *testing.T) {
	selection := workspaceSelection{
		Config: config.Config{
			Tmux: config.Tmux{SessionName: "${repo}/${branch}"},
		},
		ConfigDir:      "/Users/stan/workspace/mmb-tools/apps/sideview",
		ConfigRepoSlug: "loveholidays/sideview",
	}

	if got := sessionName(selection, "feature/example"); got != "sideview/feature-example" {
		t.Fatalf("session name = %q", got)
	}

	selection.Config.Tmux.SessionName = ""
	if got := sessionName(selection, "feature/example"); got != "sideview/feature-example" {
		t.Fatalf("default session name = %q", got)
	}
}

func TestSessionNameUsesOwnerTemplate(t *testing.T) {
	selection := workspaceSelection{
		Config: config.Config{
			Tmux: config.Tmux{SessionName: "${owner}-${repo}/${branch}"},
		},
		ConfigRepoSlug: "loveholidays/sideview",
	}

	if got := sessionName(selection, "feature/example"); got != "loveholidays-sideview/feature-example" {
		t.Fatalf("session name = %q", got)
	}
}

func TestSessionNameUsesDirectoryTemplate(t *testing.T) {
	selection := workspaceSelection{
		Config: config.Config{
			Tmux: config.Tmux{SessionName: "${dir:2}-${dir}/${branch}"},
		},
		ConfigDir:      "/Users/stan/workspace/mmb-tools/apps/sideview",
		ConfigRepoSlug: "loveholidays/sideview",
	}

	if got := sessionName(selection, "main"); got != "mmb-tools-sideview/main" {
		t.Fatalf("session name = %q", got)
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

func TestInvalidCleanupUsage(t *testing.T) {
	stderr := &bytes.Buffer{}
	status := Run([]string{"cleanup", "--bad"}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "usage: wktree cleanup") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestParseListArgs(t *testing.T) {
	parsed, err := parseListArgs([]string{"--pr"})
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.PullRequests {
		t.Fatalf("parsed = %+v", parsed)
	}
	if _, err := parseListArgs([]string{"--bad"}); err == nil || !strings.Contains(err.Error(), "usage: wktree list") {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestParseCleanupArgs(t *testing.T) {
	parsed, err := parseCleanupArgs([]string{"--dry-run", "--yes", "--workspaces"})
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.DryRun || !parsed.Yes || !parsed.Workspaces {
		t.Fatalf("parsed = %+v", parsed)
	}
	if _, err := parseCleanupArgs([]string{"extra"}); err == nil || !strings.Contains(err.Error(), "usage: wktree cleanup") {
		t.Fatalf("expected usage error, got %v", err)
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

func TestInvalidCloseUsage(t *testing.T) {
	stderr := &bytes.Buffer{}
	status := Run([]string{"close"}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "usage: wktree close") {
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

func TestParseSwitchPullRequestArgs(t *testing.T) {
	parsed, err := parseWorktreeArgs("switch", []string{"--home", "/tmp/worktrees", "--pr", "123"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.PullRequest != "123" || parsed.Home != "/tmp/worktrees" || parsed.Branch != "" {
		t.Fatalf("parsed = %+v", parsed)
	}

	parsed, err = parseWorktreeArgs("switch", []string{"--force", "--pr=https://github.com/alienxp03/demo/pull/123"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.PullRequest != "https://github.com/alienxp03/demo/pull/123" || !parsed.Force {
		t.Fatalf("parsed = %+v", parsed)
	}
}

func TestParseSwitchPullRequestRejectsBranchAndWorkspaces(t *testing.T) {
	if _, err := parseWorktreeArgs("switch", []string{"--pr", "123", "feature/example"}); err == nil || !strings.Contains(err.Error(), "usage: wktree switch") {
		t.Fatalf("expected switch usage error, got %v", err)
	}
	if _, err := parseWorktreeArgs("switch", []string{"--pr", "123", "--workspaces"}); err == nil || !strings.Contains(err.Error(), "--pr cannot be used with --workspaces") {
		t.Fatalf("expected --workspaces error, got %v", err)
	}
	if _, err := parseWorktreeArgs("new", []string{"--pr", "123"}); err == nil || !strings.Contains(err.Error(), "unknown option: --pr") {
		t.Fatalf("expected new --pr error, got %v", err)
	}
	if _, err := parseWorktreeArgs("switch", []string{"--force", "feature/example"}); err == nil || !strings.Contains(err.Error(), "--force can only be used with --pr") {
		t.Fatalf("expected --force without --pr error, got %v", err)
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
		Config:        config.Config{Tmux: config.Tmux{Mode: tmux.ModeWindow}},
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

func TestRenderWorktreeListWithPullRequests(t *testing.T) {
	worktrees := git.WorktreeList{
		CurrentPath: "/repo",
		Worktrees: []git.ListedWorktree{
			{Path: "/repo", Head: "123456789", Branch: "main"},
			{Path: "/worktrees/feature", Head: "abcdef123", Branch: "feature/pr"},
			{Path: "/worktrees/detached", Head: "deadbeef", Detached: true},
		},
	}

	withoutPR := renderWorktreeList(worktrees, nil, false)
	if strings.Contains(withoutPR, "PR") || strings.Contains(withoutPR, "https://github.com/alienxp03/demo/pull/123") {
		t.Fatalf("default list should not include PR column:\n%s", withoutPR)
	}

	withPR := renderWorktreeList(worktrees, map[string]string{"feature/pr": "https://github.com/alienxp03/demo/pull/123"}, true)
	for _, want := range []string{"CURRENT", "BRANCH", "HEAD", "PR", "PATH", "https://github.com/alienxp03/demo/pull/123", "(detached)"} {
		if !strings.Contains(withPR, want) {
			t.Fatalf("PR list output missing %q:\n%s", want, withPR)
		}
	}
}

func TestConfirmCleanupRequiresYes(t *testing.T) {
	stdout := &bytes.Buffer{}
	confirmed, err := confirmCleanup(strings.NewReader("no\n"), stdout, 2)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed {
		t.Fatal("expected no to cancel cleanup")
	}
	if !strings.Contains(stdout.String(), "Type 'yes'") {
		t.Fatalf("prompt = %q", stdout.String())
	}

	confirmed, err = confirmCleanup(strings.NewReader("yes\n"), &bytes.Buffer{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !confirmed {
		t.Fatal("expected yes to confirm cleanup")
	}
}
