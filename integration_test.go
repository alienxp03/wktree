package wktree_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWktreeNewCreatesStrictBranchAndWorktree(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(repo.root)

	result := runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "--no-setup", "--no-cd", "feature/example"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03_demo_feature-example")
	if _, err := os.Stat(worktreePath); err != nil {
		t.Fatal(err)
	}
	if got := git(t, []string{"branch", "--show-current"}, worktreePath); got != "feature/example" {
		t.Fatalf("branch = %q", got)
	}
	if got := git(t, []string{"branch", "--list", "feature/example"}, repo.sourceRoot); !strings.Contains(got, "feature/example") {
		t.Fatalf("branch list = %q", got)
	}

	result = runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "--no-setup", "--no-cd", "feature/example"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "local branch already exists: feature/example") {
		t.Fatalf("expected local branch error, status=%d stderr=%s", result.exitCode, result.stderr)
	}

	git(t, []string{"update-ref", "refs/remotes/origin/feature/remote", "HEAD"}, repo.sourceRoot)
	result = runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "--no-setup", "--no-cd", "feature/remote"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "origin branch already exists: origin/feature/remote") {
		t.Fatalf("expected origin branch error, status=%d stderr=%s", result.exitCode, result.stderr)
	}

	existingPath := filepath.Join(repo.worktreeHome, "alienxp03_demo_feature-path-exists")
	must(t, os.MkdirAll(existingPath, 0o755))
	result = runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "--no-setup", "--no-cd", "feature/path-exists"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "target worktree path already exists") {
		t.Fatalf("expected path exists error, status=%d stderr=%s", result.exitCode, result.stderr)
	}
}

func TestWktreeSwitchCreatesAndReusesExistingBranchWorktree(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(repo.root)
	sourceBranch := git(t, []string{"branch", "--show-current"}, repo.sourceRoot)

	git(t, []string{"checkout", "-b", "feature/existing"}, repo.sourceRoot)
	write(t, filepath.Join(repo.sourceRoot, "branch.txt"), "existing branch\n")
	git(t, []string{"add", "branch.txt"}, repo.sourceRoot)
	git(t, []string{"commit", "-m", "Existing branch commit"}, repo.sourceRoot)
	existingCommit := git(t, []string{"rev-parse", "HEAD"}, repo.sourceRoot)
	git(t, []string{"checkout", sourceBranch}, repo.sourceRoot)

	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), "postSetup:\n  - printf switched > switched.txt\n")
	result := runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "--no-cd", "feature/existing"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03_demo_feature-existing")
	if got := git(t, []string{"branch", "--show-current"}, worktreePath); got != "feature/existing" {
		t.Fatalf("branch = %q", got)
	}
	if got := git(t, []string{"rev-parse", "HEAD"}, worktreePath); got != existingCommit {
		t.Fatalf("HEAD = %q, want %q", got, existingCommit)
	}
	if got := read(t, filepath.Join(worktreePath, "switched.txt")); got != "switched" {
		t.Fatalf("switched.txt = %q", got)
	}

	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), "postSetup:\n  - printf rerun > rerun.txt\n")
	result = runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "--no-cd", "feature/existing"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("reuse status=%d stderr=%s", result.exitCode, result.stderr)
	}
	if _, err := os.Stat(filepath.Join(worktreePath, "rerun.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected setup to be skipped on reused worktree, stat err=%v", err)
	}
}

func TestWktreeSwitchCreatesTrackingBranchFromOrigin(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(repo.root)

	git(t, []string{"update-ref", "refs/remotes/origin/feature/remote", "HEAD"}, repo.sourceRoot)
	result := runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "--no-setup", "--no-cd", "origin/feature/remote"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03_demo_feature-remote")
	if got := git(t, []string{"branch", "--show-current"}, worktreePath); got != "feature/remote" {
		t.Fatalf("branch = %q", got)
	}
	if got := git(t, []string{"rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"}, worktreePath); got != "origin/feature/remote" {
		t.Fatalf("upstream = %q", got)
	}
}

func TestWktreeSwitchRequiresExistingBranch(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(repo.root)

	result := runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "--no-setup", "--no-cd", "feature/missing"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "branch does not exist locally or on origin: feature/missing") {
		t.Fatalf("expected missing branch error, status=%d stderr=%s", result.exitCode, result.stderr)
	}
}

func TestWktreeListShowsAllWorktrees(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(repo.root)
	sourceBranch := git(t, []string{"branch", "--show-current"}, repo.sourceRoot)

	result := runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "--no-setup", "--no-cd", "feature/listed"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("new status=%d stderr=%s", result.exitCode, result.stderr)
	}
	detachedPath := filepath.Join(repo.worktreeHome, "detached")
	git(t, []string{"worktree", "add", "--detach", detachedPath, "HEAD"}, repo.sourceRoot)

	result = runWktree(t, binary, []string{"list"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("list status=%d stderr=%s", result.exitCode, result.stderr)
	}
	for _, want := range []string{"CURRENT", "BRANCH", "HEAD", "PATH", sourceBranch, "feature/listed", "(detached)", "alienxp03_demo_feature-listed", "detached"} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("list output missing %q:\n%s", want, result.stdout)
		}
	}
	if !hasCurrentBranchLine(result.stdout, sourceBranch) {
		t.Fatalf("list output missing current marker for %s:\n%s", sourceBranch, result.stdout)
	}
}

func TestWktreeRemoveDeletesWorktreeAndBranch(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(repo.root)

	result := runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "--no-setup", "--no-cd", "feature/remove"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("new status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03_demo_feature-remove")

	result = runWktree(t, binary, []string{"__complete", "remove", "feature/r"}, repo.sourceRoot, env)
	if result.exitCode != 0 || !strings.Contains(result.stdout, "feature/remove") {
		t.Fatalf("remove completion status=%d stdout=%q stderr=%s", result.exitCode, result.stdout, result.stderr)
	}

	result = runWktree(t, binary, []string{"remove", "feature/remove"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("remove status=%d stderr=%s", result.exitCode, result.stderr)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("expected worktree removed, stat err=%v", err)
	}
	if got := git(t, []string{"branch", "--list", "feature/remove"}, repo.sourceRoot); got != "" {
		t.Fatalf("branch still exists: %q", got)
	}
}

func TestWktreeRemoveRejectsCurrentWorktree(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(repo.root)
	sourceBranch := git(t, []string{"branch", "--show-current"}, repo.sourceRoot)

	result := runWktree(t, binary, []string{"remove", sourceBranch}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "cannot remove current worktree") {
		t.Fatalf("expected current worktree error, status=%d stderr=%s", result.exitCode, result.stderr)
	}
}

func TestWktreeRemoveForceDeletesUnmergedBranch(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(repo.root)
	sourceBranch := git(t, []string{"branch", "--show-current"}, repo.sourceRoot)

	git(t, []string{"checkout", "-b", "feature/unmerged"}, repo.sourceRoot)
	write(t, filepath.Join(repo.sourceRoot, "unmerged.txt"), "unmerged\n")
	git(t, []string{"add", "unmerged.txt"}, repo.sourceRoot)
	git(t, []string{"commit", "-m", "Unmerged commit"}, repo.sourceRoot)
	git(t, []string{"checkout", sourceBranch}, repo.sourceRoot)

	result := runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "--no-setup", "--no-cd", "feature/unmerged"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("switch status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03_demo_feature-unmerged")

	result = runWktree(t, binary, []string{"remove", "feature/unmerged"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "branch is not merged into current HEAD") {
		t.Fatalf("expected unmerged error, status=%d stderr=%s", result.exitCode, result.stderr)
	}

	result = runWktree(t, binary, []string{"remove", "--force", "feature/unmerged"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("force remove status=%d stderr=%s", result.exitCode, result.stderr)
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("expected worktree removed, stat err=%v", err)
	}
	if got := git(t, []string{"branch", "--list", "feature/unmerged"}, repo.sourceRoot); got != "" {
		t.Fatalf("branch still exists: %q", got)
	}
}

func TestWktreeCompletesSwitchBranches(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(repo.root)

	git(t, []string{"branch", "feature/local"}, repo.sourceRoot)
	git(t, []string{"update-ref", "refs/remotes/origin/feature/remote", "HEAD"}, repo.sourceRoot)

	result := runWktree(t, binary, []string{"__complete", "switch", "feature/"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("complete status=%d stderr=%s", result.exitCode, result.stderr)
	}
	for _, want := range []string{"feature/local", "feature/remote"} {
		if !strings.Contains(result.stdout, want+"\n") {
			t.Fatalf("completion missing %q: %q", want, result.stdout)
		}
	}
}

func TestWktreeNewAppliesSetupConfig(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(repo.root)
	globalConfigDir := filepath.Join(envValue(env, "XDG_CONFIG_HOME"), "wktree")
	must(t, os.MkdirAll(globalConfigDir, 0o755))
	write(t, filepath.Join(repo.sourceRoot, ".env"), "GLOBAL_ENV=1\n")
	write(t, filepath.Join(repo.sourceRoot, ".env.local"), "PROJECT_ENV=1\n")
	write(t, filepath.Join(repo.sourceRoot, ".mcp.json"), "{\"global\":true}\n")
	write(t, filepath.Join(repo.sourceRoot, ".tool-versions"), "go 1.26.1\n")
	write(t, filepath.Join(globalConfigDir, "config.yaml"), strings.Join([]string{
		"copy:",
		"  - .env",
		"  - .missing",
		"symlink:",
		"  - .mcp.json",
		"  - .missing-link",
		"postSetup:",
		"  - printf global > global.txt",
		"",
	}, "\n"))
	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"copy:",
		"  - .env.local",
		"symlink:",
		"  - .tool-versions",
		"postSetup:",
		"  - printf project > project.txt",
		"",
	}, "\n"))

	result := runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "--no-cd", "feature/setup"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03_demo_feature-setup")
	globalLink := filepath.Join(worktreePath, ".mcp.json")
	projectLink := filepath.Join(worktreePath, ".tool-versions")
	if got := read(t, filepath.Join(worktreePath, ".env")); got != "GLOBAL_ENV=1\n" {
		t.Fatalf(".env = %q", got)
	}
	if got := read(t, filepath.Join(worktreePath, ".env.local")); got != "PROJECT_ENV=1\n" {
		t.Fatalf(".env.local = %q", got)
	}
	assertSymlinkTarget(t, globalLink, filepath.Join(repo.sourceRoot, ".mcp.json"))
	assertSymlinkTarget(t, projectLink, filepath.Join(repo.sourceRoot, ".tool-versions"))
	if got := read(t, filepath.Join(worktreePath, "global.txt")); got != "global" {
		t.Fatalf("global.txt = %q", got)
	}
	if got := read(t, filepath.Join(worktreePath, "project.txt")); got != "project" {
		t.Fatalf("project.txt = %q", got)
	}
	if !strings.Contains(result.stderr, "copy source not found, skipping: .missing") {
		t.Fatalf("stderr missing copy warning: %s", result.stderr)
	}
	if !strings.Contains(result.stderr, "symlink source not found, skipping: .missing-link") {
		t.Fatalf("stderr missing symlink warning: %s", result.stderr)
	}
}

func TestFailingPostSetupLeavesWorktreeIntact(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(repo.root)
	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), "postSetup:\n  - exit 1\n")

	result := runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "--no-cd", "feature/failsetup"}, repo.sourceRoot, env)
	if result.exitCode != 1 || !strings.Contains(result.stderr, "post setup command failed") {
		t.Fatalf("expected setup failure, status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03_demo_feature-failsetup")
	if _, err := os.Stat(worktreePath); err != nil {
		t.Fatal(err)
	}
	if got := git(t, []string{"branch", "--show-current"}, worktreePath); got != "feature/failsetup" {
		t.Fatalf("branch = %q", got)
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "wktree")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/wktree")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, output)
	}
	return binary
}

type tempRepo struct {
	root         string
	sourceRoot   string
	worktreeHome string
}

func createTempRepo(t *testing.T) tempRepo {
	t.Helper()
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	must(t, os.MkdirAll(sourceRoot, 0o755))
	git(t, []string{"init"}, sourceRoot)
	git(t, []string{"config", "user.email", "test@example.com"}, sourceRoot)
	git(t, []string{"config", "user.name", "Test User"}, sourceRoot)
	write(t, filepath.Join(sourceRoot, "README.md"), "test\n")
	git(t, []string{"add", "README.md"}, sourceRoot)
	git(t, []string{"commit", "-m", "Initial commit"}, sourceRoot)
	git(t, []string{"remote", "add", "origin", "git@github.com:alienxp03/demo.git"}, sourceRoot)
	return tempRepo{root: root, sourceRoot: sourceRoot, worktreeHome: filepath.Join(root, "worktrees")}
}

type commandResult struct {
	exitCode int
	stdout   string
	stderr   string
}

func runWktree(t *testing.T, binary string, args []string, cwd string, env []string) commandResult {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = cwd
	cmd.Env = env
	stdout, stderr := strings.Builder{}, strings.Builder{}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("wktree failed: %v", err)
		}
	}
	return commandResult{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}

func git(t *testing.T, args []string, cwd string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func testEnv(root string) []string {
	return append(os.Environ(),
		"HOME="+filepath.Join(root, "home"),
		"XDG_CONFIG_HOME="+filepath.Join(root, "xdg"),
	)
}

func envValue(env []string, key string) string {
	for _, item := range env {
		k, v, ok := strings.Cut(item, "=")
		if ok && k == key {
			return v
		}
	}
	return ""
}

func assertSymlinkTarget(t *testing.T, linkPath string, wantPath string) {
	t.Helper()
	stat, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", linkPath)
	}
	got, err := filepath.EvalSymlinks(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("symlink target = %q, want %q", got, want)
	}
}

func hasCurrentBranchLine(output string, branch string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "*" && fields[1] == branch {
			return true
		}
	}
	return false
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
