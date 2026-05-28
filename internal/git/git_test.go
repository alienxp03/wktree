package git

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alienxp03/wktree/internal/run"
)

func TestRepoSlugUsesGitHubRemote(t *testing.T) {
	got, err := RepoSlug(context.Background(), "/tmp/workspace/wktree", run.RunnerFunc(func(_ context.Context, _ string, args []string, _ run.Options) run.Result {
		if strings.Join(args, " ") == "config --get remote.origin.url" {
			return run.Result{Stdout: "git@github.com:alienxp03/wktree.git\n"}
		}
		return run.Result{ExitCode: 1}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join("alienxp03", "wktree") {
		t.Fatalf("repo slug = %q", got)
	}
}

func TestRepoSlugFallsBackToUserName(t *testing.T) {
	got, err := RepoSlug(context.Background(), "/tmp/workspace/tree_1", run.RunnerFunc(func(_ context.Context, _ string, args []string, _ run.Options) run.Result {
		switch strings.Join(args, " ") {
		case "config --get remote.origin.url":
			return run.Result{ExitCode: 1}
		case "config --get user.name":
			return run.Result{Stdout: "Test User\n"}
		default:
			return run.Result{ExitCode: 1}
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join("Test-User", "tree_1") {
		t.Fatalf("repo slug = %q", got)
	}
}

func TestRepoSlugRequiresUserNameWithoutGitHubRemote(t *testing.T) {
	_, err := RepoSlug(context.Background(), "/tmp/workspace/tree_1", run.RunnerFunc(func(_ context.Context, _ string, _ []string, _ run.Options) run.Result {
		return run.Result{ExitCode: 1}
	}))
	if err == nil || !strings.Contains(err.Error(), "user.name") {
		t.Fatalf("err = %v", err)
	}
}

func TestNormalizePullRequestValue(t *testing.T) {
	for _, input := range []string{"123", "https://github.com/alienxp03/demo/pull/123"} {
		got, err := normalizePullRequestValue(input)
		if err != nil {
			t.Fatalf("normalizePullRequestValue(%q) returned error: %v", input, err)
		}
		if got != input {
			t.Fatalf("normalizePullRequestValue(%q) = %q", input, got)
		}
	}
	if _, err := normalizePullRequestValue("feature/example"); err == nil {
		t.Fatal("normalizePullRequestValue should reject non-numeric non-URL values")
	}
}

func TestPullRequestURLsByBranch(t *testing.T) {
	got, err := PullRequestURLsByBranch(context.Background(), "/repo", []string{"feature/one", "feature/missing"}, run.RunnerFunc(func(_ context.Context, command string, args []string, options run.Options) run.Result {
		if command != "gh" {
			t.Fatalf("command = %q", command)
		}
		if options.Cwd != "/repo" {
			t.Fatalf("cwd = %q", options.Cwd)
		}
		if strings.Join(args, " ") != "pr list --state open --limit 100 --json headRefName,url" {
			t.Fatalf("args = %v", args)
		}
		return run.Result{Stdout: `[{"headRefName":"feature/one","url":"https://github.com/alienxp03/demo/pull/1"},{"headRefName":"other","url":"https://github.com/alienxp03/demo/pull/2"}]`}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got["feature/one"] != "https://github.com/alienxp03/demo/pull/1" {
		t.Fatalf("urls = %#v", got)
	}
	if _, ok := got["feature/missing"]; ok {
		t.Fatalf("missing branch should not have URL: %#v", got)
	}
}

func TestGeneratedStatusLineMatchesNestedContextEnv(t *testing.T) {
	for _, line := range []string{"?? .wktree.env", "?? projects/project_a/.wktree.env", " M apps/app_a/.wktree.env"} {
		if !isGeneratedStatusLine(line) {
			t.Fatalf("expected generated status line: %q", line)
		}
	}
	if isGeneratedStatusLine("?? .env") {
		t.Fatal(".env should not be treated as generated")
	}
}
