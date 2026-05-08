package setup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alienxp03/wktree/internal/config"
	"github.com/alienxp03/wktree/internal/run"
)

func TestPlanRoundTrip(t *testing.T) {
	root := t.TempDir()
	planPath := filepath.Join(root, "plan.json")
	plan := NewPlan(filepath.Join(root, "source"), filepath.Join(root, "worktree"), config.Config{
		Copy:      []string{".env"},
		Symlink:   []string{".mcp.json"},
		PostSetup: []string{"pnpm install"},
	})

	if err := WritePlan(planPath, plan); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPlan(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoRoot != plan.RepoRoot || got.WorktreePath != plan.WorktreePath {
		t.Fatalf("plan mismatch: %#v", got)
	}
	assertSlice(t, got.Copy, plan.Copy)
	assertSlice(t, got.Symlink, plan.Symlink)
	assertSlice(t, got.PostSetup, plan.PostSetup)
}

func TestCopyFiles(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "source")
	worktreePath := filepath.Join(root, "worktree")
	must(t, os.MkdirAll(repoRoot, 0o755))
	must(t, os.MkdirAll(worktreePath, 0o755))
	write(t, filepath.Join(repoRoot, ".env"), "SECRET=1\n")
	logger, stdout, stderr := captureLogger()

	status := CopyFiles(Plan{
		RepoRoot:     repoRoot,
		WorktreePath: worktreePath,
		Copy:         []string{".env", ".missing"},
	}, logger)

	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if got := read(t, filepath.Join(worktreePath, ".env")); got != "SECRET=1\n" {
		t.Fatalf("copied content = %q", got)
	}
	if !strings.Contains(stdout.String(), "copied .env") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "copy source not found") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCopyFilesDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "source")
	worktreePath := filepath.Join(root, "worktree")
	must(t, os.MkdirAll(repoRoot, 0o755))
	must(t, os.MkdirAll(worktreePath, 0o755))
	write(t, filepath.Join(repoRoot, ".env"), "new\n")
	write(t, filepath.Join(worktreePath, ".env"), "existing\n")
	logger, _, stderr := captureLogger()

	status := CopyFiles(Plan{RepoRoot: repoRoot, WorktreePath: worktreePath, Copy: []string{".env"}}, logger)

	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if got := read(t, filepath.Join(worktreePath, ".env")); got != "existing\n" {
		t.Fatalf("destination overwritten: %q", got)
	}
	if !strings.Contains(stderr.String(), "copy destination already exists") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestSymlinkFiles(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "source")
	worktreePath := filepath.Join(root, "worktree")
	must(t, os.MkdirAll(repoRoot, 0o755))
	must(t, os.MkdirAll(worktreePath, 0o755))
	write(t, filepath.Join(repoRoot, ".mcp.json"), "{\"ok\":true}\n")
	logger, stdout, stderr := captureLogger()

	status := SymlinkFiles(Plan{
		RepoRoot:     repoRoot,
		WorktreePath: worktreePath,
		Symlink:      []string{".mcp.json", ".missing"},
	}, logger)

	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	linkPath := filepath.Join(worktreePath, ".mcp.json")
	stat, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected symlink")
	}
	if got := read(t, linkPath); got != "{\"ok\":true}\n" {
		t.Fatalf("linked content = %q", got)
	}
	if !strings.Contains(stdout.String(), "symlinked .mcp.json") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "symlink source not found") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestSymlinkFilesDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	repoRoot := filepath.Join(root, "source")
	worktreePath := filepath.Join(root, "worktree")
	must(t, os.MkdirAll(repoRoot, 0o755))
	must(t, os.MkdirAll(worktreePath, 0o755))
	write(t, filepath.Join(repoRoot, ".mcp.json"), "source\n")
	write(t, filepath.Join(worktreePath, ".mcp.json"), "existing\n")
	logger, _, stderr := captureLogger()

	status := SymlinkFiles(Plan{RepoRoot: repoRoot, WorktreePath: worktreePath, Symlink: []string{".mcp.json"}}, logger)

	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if got := read(t, filepath.Join(worktreePath, ".mcp.json")); got != "existing\n" {
		t.Fatalf("destination overwritten: %q", got)
	}
	if !strings.Contains(stderr.String(), "symlink destination already exists") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunPostSetupStopsOnFailure(t *testing.T) {
	root := t.TempDir()
	worktreePath := filepath.Join(root, "worktree")
	must(t, os.MkdirAll(worktreePath, 0o755))
	logger, _, stderr := captureLogger()
	calls := []string{}

	status := Run(context.Background(), Plan{
		RepoRoot:     root,
		WorktreePath: worktreePath,
		PostSetup:    []string{"echo ok", "false", "echo later"},
	}, logger, ShellRunnerFunc(func(_ context.Context, command string, cwd string, inherit bool) run.Result {
		calls = append(calls, command+"@"+cwd)
		if command == "false" {
			return run.Result{ExitCode: 1}
		}
		return run.Result{ExitCode: 0}
	}))

	if status != 1 {
		t.Fatalf("status = %d", status)
	}
	assertSlice(t, calls, []string{"echo ok@" + worktreePath, "false@" + worktreePath})
	if !strings.Contains(stderr.String(), "post setup command failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func captureLogger() (Logger, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	return Logger{Stdout: stdout, Stderr: stderr}, stdout, stderr
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

func write(t *testing.T, filePath string, content string) {
	t.Helper()
	must(t, os.WriteFile(filePath, []byte(content), 0o644))
}

func read(t *testing.T, filePath string) string {
	t.Helper()
	data, err := os.ReadFile(filePath)
	must(t, err)
	return string(data)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
