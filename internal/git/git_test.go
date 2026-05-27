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
