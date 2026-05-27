package wktree_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestWktreeNewCreatesStrictBranchAndWorktree(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)

	result := runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "feature/example"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-example")
	if _, err := os.Stat(worktreePath); err != nil {
		t.Fatal(err)
	}
	if got := git(t, []string{"branch", "--show-current"}, worktreePath); got != "feature/example" {
		t.Fatalf("branch = %q", got)
	}
	if got := git(t, []string{"branch", "--list", "feature/example"}, repo.sourceRoot); !strings.Contains(got, "feature/example") {
		t.Fatalf("branch list = %q", got)
	}

	result = runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "feature/example"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "local branch already exists: feature/example") {
		t.Fatalf("expected local branch error, status=%d stderr=%s", result.exitCode, result.stderr)
	}

	git(t, []string{"update-ref", "refs/remotes/origin/feature/remote", "HEAD"}, repo.sourceRoot)
	result = runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "feature/remote"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "origin branch already exists: origin/feature/remote") {
		t.Fatalf("expected origin branch error, status=%d stderr=%s", result.exitCode, result.stderr)
	}

	existingPath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-path-exists")
	must(t, os.MkdirAll(existingPath, 0o755))
	result = runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "feature/path-exists"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "target worktree path already exists") {
		t.Fatalf("expected path exists error, status=%d stderr=%s", result.exitCode, result.stderr)
	}
}

func TestWktreeNewFromCreatesBranchFromStartPoint(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)
	sourceBranch := git(t, []string{"branch", "--show-current"}, repo.sourceRoot)

	git(t, []string{"checkout", "-b", "base/from"}, repo.sourceRoot)
	write(t, filepath.Join(repo.sourceRoot, "from.txt"), "from branch\n")
	git(t, []string{"add", "from.txt"}, repo.sourceRoot)
	git(t, []string{"commit", "-m", "Base from commit"}, repo.sourceRoot)
	baseCommit := git(t, []string{"rev-parse", "HEAD"}, repo.sourceRoot)
	git(t, []string{"checkout", sourceBranch}, repo.sourceRoot)

	result := runWktree(t, binary, []string{"new", "--from", "base/from", "--home", repo.worktreeHome, "feature/from"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-from")
	if got := git(t, []string{"branch", "--show-current"}, worktreePath); got != "feature/from" {
		t.Fatalf("branch = %q", got)
	}
	if got := git(t, []string{"rev-parse", "HEAD"}, worktreePath); got != baseCommit {
		t.Fatalf("HEAD = %q, want %q", got, baseCommit)
	}
	if got := read(t, filepath.Join(worktreePath, "from.txt")); got != "from branch\n" {
		t.Fatalf("from.txt = %q", got)
	}
}

func TestWktreeNewUsesUserNameForRepoWithoutRemote(t *testing.T) {
	binary := buildBinary(t)
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "tree_1")
	worktreeHome := filepath.Join(root, "worktrees")
	must(t, os.MkdirAll(sourceRoot, 0o755))
	git(t, []string{"init"}, sourceRoot)
	git(t, []string{"config", "user.email", "test@example.com"}, sourceRoot)
	git(t, []string{"config", "user.name", "alienxp03"}, sourceRoot)
	write(t, filepath.Join(sourceRoot, "README.md"), "local\n")
	git(t, []string{"add", "README.md"}, sourceRoot)
	git(t, []string{"commit", "-m", "Initial commit"}, sourceRoot)
	env := testEnv(t, root)

	result := runWktree(t, binary, []string{"new", "--home", worktreeHome, "feature/testing_1"}, sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(worktreeHome, "alienxp03", "tree_1", "feature-testing_1")
	if got := git(t, []string{"branch", "--show-current"}, worktreePath); got != "feature/testing_1" {
		t.Fatalf("branch = %q", got)
	}
}

func TestWktreeSwitchCreatesAndReusesExistingBranchWorktree(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)
	sourceBranch := git(t, []string{"branch", "--show-current"}, repo.sourceRoot)

	git(t, []string{"checkout", "-b", "feature/existing"}, repo.sourceRoot)
	write(t, filepath.Join(repo.sourceRoot, "branch.txt"), "existing branch\n")
	git(t, []string{"add", "branch.txt"}, repo.sourceRoot)
	git(t, []string{"commit", "-m", "Existing branch commit"}, repo.sourceRoot)
	existingCommit := git(t, []string{"rev-parse", "HEAD"}, repo.sourceRoot)
	git(t, []string{"checkout", sourceBranch}, repo.sourceRoot)

	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"tmux_mode: window",
		"workspaces:",
		"  - name: app",
		"    hooks:",
		"      post_create:",
		"        - printf switched > switched.txt",
		"",
	}, "\n"))
	result := runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "feature/existing"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-existing")
	if got := git(t, []string{"branch", "--show-current"}, worktreePath); got != "feature/existing" {
		t.Fatalf("branch = %q", got)
	}
	if got := git(t, []string{"rev-parse", "HEAD"}, worktreePath); got != existingCommit {
		t.Fatalf("HEAD = %q, want %q", got, existingCommit)
	}
	if got := read(t, filepath.Join(worktreePath, "switched.txt")); got != "switched" {
		t.Fatalf("switched.txt = %q", got)
	}

	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"tmux_mode: window",
		"workspaces:",
		"  - name: app",
		"    hooks:",
		"      post_create:",
		"        - printf rerun > rerun.txt",
		"",
	}, "\n"))
	result = runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "feature/existing"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("reuse status=%d stderr=%s", result.exitCode, result.stderr)
	}
	if got := read(t, filepath.Join(worktreePath, "rerun.txt")); got != "rerun" {
		t.Fatalf("rerun.txt = %q", got)
	}
}

func TestWktreeSwitchCreatesTrackingBranchFromOrigin(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)

	git(t, []string{"update-ref", "refs/remotes/origin/feature/remote", "HEAD"}, repo.sourceRoot)
	result := runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "origin/feature/remote"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-remote")
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
	env := testEnv(t, repo.root)

	result := runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "feature/missing"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "branch does not exist locally or on origin: feature/missing") {
		t.Fatalf("expected missing branch error, status=%d stderr=%s", result.exitCode, result.stderr)
	}
}

func TestWktreeSwitchPullRequestCreatesAndUpdatesSingleRepoWorktree(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	createSiblingRepo(t, repo.root, "frontend", "frontend")
	env := testEnv(t, repo.root)
	enableLocalOrigin(t, repo)
	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"worktree_dir: " + repo.worktreeHome,
		"workspace_mode: all",
		"workspaces:",
		"  - name: backend",
		"    hooks:",
		"      post_create:",
		"        - printf prsetup > prsetup.txt",
		"  - name: frontend",
		"    repo: ../frontend",
		"    hooks:",
		"      post_create:",
		"        - printf frontend > frontend.txt",
		"",
	}, "\n"))
	firstHead := pushPullRefCommit(t, repo, 123, "pr.txt", "first\n")
	writeFakeGh(t, env, pullRequestJSON(123, "contributor/feature", firstHead, "https://github.com/alienxp03/demo/pull/123"))
	result := runWktree(t, binary, []string{"switch", "--pr", "123"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("switch --pr status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "contributor-feature")
	if got := git(t, []string{"branch", "--show-current"}, worktreePath); got != "contributor/feature" {
		t.Fatalf("branch = %q", got)
	}
	if got := git(t, []string{"rev-parse", "HEAD"}, worktreePath); got != firstHead {
		t.Fatalf("HEAD = %q, want %q", got, firstHead)
	}
	if got := read(t, filepath.Join(worktreePath, "prsetup.txt")); got != "prsetup" {
		t.Fatalf("prsetup.txt = %q", got)
	}
	envFile := read(t, filepath.Join(worktreePath, ".wktree.env"))
	for _, want := range []string{"WKTREE_BACKEND_DIR=", "WKTREE_PR_NUMBER='123'", "WKTREE_PR_URL='https://github.com/alienxp03/demo/pull/123'", "WKTREE_PR_HEAD_REF='contributor/feature'"} {
		if !strings.Contains(envFile, want) {
			t.Fatalf("PR env missing %q:\n%s", want, envFile)
		}
	}
	if strings.Contains(envFile, "WKTREE_FRONTEND_DIR=") {
		t.Fatalf("PR mode should not include frontend workspace:\n%s", envFile)
	}
	frontendPath := filepath.Join(repo.worktreeHome, "alienxp03", "frontend", "contributor-feature")
	if _, err := os.Stat(frontendPath); !os.IsNotExist(err) {
		t.Fatalf("PR mode should not create frontend worktree, stat err=%v", err)
	}

	secondHead := pushPullRefCommit(t, repo, 123, "pr.txt", "second\n")
	writeFakeGh(t, env, pullRequestJSON(123, "contributor/feature", secondHead, "https://github.com/alienxp03/demo/pull/123"))
	result = runWktree(t, binary, []string{"switch", "--pr=https://github.com/alienxp03/demo/pull/123"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("reuse switch --pr status=%d stderr=%s", result.exitCode, result.stderr)
	}
	if got := git(t, []string{"rev-parse", "HEAD"}, worktreePath); got != secondHead {
		t.Fatalf("reused HEAD = %q, want %q", got, secondHead)
	}
	if got := read(t, filepath.Join(worktreePath, "pr.txt")); got != "second\n" {
		t.Fatalf("pr.txt = %q", got)
	}
}

func TestWktreeSwitchPullRequestRejectsRepoMismatch(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)
	enableLocalOrigin(t, repo)
	head := pushPullRefCommit(t, repo, 123, "pr.txt", "mismatch\n")
	writeFakeGh(t, env, pullRequestJSON(123, "contributor/feature", head, "https://github.com/other/demo/pull/123"))

	result := runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "--pr", "123"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "does not match current repo") {
		t.Fatalf("expected repo mismatch, status=%d stderr=%s", result.exitCode, result.stderr)
	}
}

func TestWktreeSwitchPullRequestRejectsBranchCollision(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)
	enableLocalOrigin(t, repo)
	head := pushPullRefCommit(t, repo, 123, "pr.txt", "collision\n")
	writeFakeGh(t, env, pullRequestJSON(123, "feature/collision", head, "https://github.com/alienxp03/demo/pull/123"))
	git(t, []string{"branch", "feature/collision"}, repo.sourceRoot)

	result := runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "--pr", "123"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "local branch already exists") {
		t.Fatalf("expected branch collision, status=%d stderr=%s", result.exitCode, result.stderr)
	}
}

func TestWktreeListShowsAllWorktrees(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)
	sourceBranch := git(t, []string{"branch", "--show-current"}, repo.sourceRoot)

	result := runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "feature/listed"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("new status=%d stderr=%s", result.exitCode, result.stderr)
	}
	detachedPath := filepath.Join(repo.worktreeHome, "detached")
	git(t, []string{"worktree", "add", "--detach", detachedPath, "HEAD"}, repo.sourceRoot)

	result = runWktree(t, binary, []string{"list"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("list status=%d stderr=%s", result.exitCode, result.stderr)
	}
	for _, want := range []string{"CURRENT", "BRANCH", "HEAD", "PATH", sourceBranch, "feature/listed", "(detached)", filepath.Join("alienxp03", "demo", "feature-listed"), "detached"} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("list output missing %q:\n%s", want, result.stdout)
		}
	}
	if !hasCurrentBranchLine(result.stdout, sourceBranch) {
		t.Fatalf("list output missing current marker for %s:\n%s", sourceBranch, result.stdout)
	}
}

func TestWktreeDoctorReportsRepository(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)

	result := runWktree(t, binary, []string{"doctor"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("doctor status=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
	for _, want := range []string{"[ok] repo root:", "[ok] repo slug:", "[ok] worktrees:", "[ok] config:"} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, result.stdout)
		}
	}
}

func TestWktreeInitCreatesProjectConfig(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)

	result := runWktree(t, binary, []string{"init"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("init status=%d stderr=%s", result.exitCode, result.stderr)
	}
	configPath := filepath.Join(repo.sourceRoot, ".wktree.yaml")
	if !strings.HasPrefix(result.stdout, "created ") || !strings.HasSuffix(strings.TrimSpace(result.stdout), string(filepath.Separator)+".wktree.yaml") {
		t.Fatalf("stdout = %q", result.stdout)
	}
	config := read(t, configPath)
	for _, want := range []string{"# worktree_dir: ~/workspace/worktrees", "# tmux_mode: window", "# workspace_mode: single", "# randomize_ports:", "#       - PORT", "name: window_name", "repo: ."} {
		if !strings.Contains(config, want) {
			t.Fatalf("config missing %q:\n%s", want, config)
		}
	}

	result = runWktree(t, binary, []string{"init"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "config already exists") {
		t.Fatalf("expected existing config error, status=%d stderr=%s", result.exitCode, result.stderr)
	}

	projectRoot := filepath.Join(repo.sourceRoot, "projects", "project_a")
	must(t, os.MkdirAll(projectRoot, 0o755))
	result = runWktree(t, binary, []string{"init"}, projectRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("project init status=%d stderr=%s", result.exitCode, result.stderr)
	}
	projectConfigPath := filepath.Join(projectRoot, ".wktree.yaml")
	if _, err := os.Stat(projectConfigPath); err != nil {
		t.Fatalf("expected project config in opened folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo.sourceRoot, "projects", ".wktree.yaml")); !os.IsNotExist(err) {
		t.Fatalf("did not expect config in parent project folder, stat err=%v", err)
	}
}

func TestWktreeRemoveDeletesWorktreeAndBranch(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)

	result := runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "feature/remove"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("new status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-remove")

	result = runWktree(t, binary, []string{"__complete", "remove", "feature/r"}, repo.sourceRoot, env)
	if result.exitCode != 0 || !strings.Contains(result.stdout, "feature/remove") {
		t.Fatalf("remove completion status=%d stdout=%q stderr=%s", result.exitCode, result.stdout, result.stderr)
	}

	result = runWktree(t, binary, []string{"remove", "--dry-run", "feature/remove"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("dry-run remove status=%d stderr=%s", result.exitCode, result.stderr)
	}
	for _, want := range []string{"dry run: remove", "tmux mode:", "tmux kill-window", "git worktree remove", "git branch -d feature/remove"} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, result.stdout)
		}
	}
	if _, err := os.Stat(worktreePath); err != nil {
		t.Fatalf("expected dry-run to keep worktree, stat err=%v", err)
	}
	if got := git(t, []string{"branch", "--list", "feature/remove"}, repo.sourceRoot); !strings.Contains(got, "feature/remove") {
		t.Fatalf("expected dry-run to keep branch, branch list=%q", got)
	}

	result = runWktree(t, binary, []string{"remove", "feature/remove"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("remove status=%d stderr=%s", result.exitCode, result.stderr)
	}
	for _, want := range []string{
		"remove: alienxp03/demo: checking clean worktree",
		"remove: alienxp03/demo: cleaning generated workspace env",
		"remove: closing tmux targets",
		"remove: alienxp03/demo: removing git worktree",
		"remove: alienxp03/demo: deleting local branch",
		"removed feature/remove",
	} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("remove output missing %q:\n%s", want, result.stdout)
		}
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("expected worktree removed, stat err=%v", err)
	}
	if got := git(t, []string{"branch", "--list", "feature/remove"}, repo.sourceRoot); got != "" {
		t.Fatalf("branch still exists: %q", got)
	}
}

func TestWktreeCloseKeepsWorktreeAndBranch(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)

	result := runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "feature/close"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("new status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-close")

	result = runWktree(t, binary, []string{"__complete", "close", "feature/c"}, repo.sourceRoot, env)
	if result.exitCode != 0 || !strings.Contains(result.stdout, "feature/close") {
		t.Fatalf("close completion status=%d stdout=%q stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
	result = runWktree(t, binary, []string{"__complete", "close", "feature/c"}, worktreePath, env)
	if result.exitCode != 0 || !strings.Contains(result.stdout, "feature/close") {
		t.Fatalf("close current worktree completion status=%d stdout=%q stderr=%s", result.exitCode, result.stdout, result.stderr)
	}

	result = runWktree(t, binary, []string{"close", "--dry-run", "feature/close"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("dry-run close status=%d stderr=%s", result.exitCode, result.stderr)
	}
	for _, want := range []string{"dry run: close", "tmux mode:", "tmux kill-window"} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("dry-run close output missing %q:\n%s", want, result.stdout)
		}
	}
	for _, notWant := range []string{"git worktree remove", "git branch -d"} {
		if strings.Contains(result.stdout, notWant) {
			t.Fatalf("dry-run close output should not include %q:\n%s", notWant, result.stdout)
		}
	}

	result = runWktree(t, binary, []string{"close", "feature/close"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("close status=%d stderr=%s", result.exitCode, result.stderr)
	}
	for _, want := range []string{"close: closing tmux targets", "closed feature/close"} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("close output missing %q:\n%s", want, result.stdout)
		}
	}
	if _, err := os.Stat(worktreePath); err != nil {
		t.Fatalf("expected close to keep worktree, stat err=%v", err)
	}
	if got := git(t, []string{"branch", "--list", "feature/close"}, repo.sourceRoot); !strings.Contains(got, "feature/close") {
		t.Fatalf("expected close to keep branch, branch list=%q", got)
	}
	if _, err := os.Stat(filepath.Join(worktreePath, ".wktree.env")); err != nil {
		t.Fatalf("expected close to keep .wktree.env, stat err=%v", err)
	}
}

func TestWktreeRemoveRejectsCurrentWorktree(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)
	sourceBranch := git(t, []string{"branch", "--show-current"}, repo.sourceRoot)

	result := runWktree(t, binary, []string{"remove", sourceBranch}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "cannot remove current worktree") {
		t.Fatalf("expected current worktree error, status=%d stderr=%s", result.exitCode, result.stderr)
	}
}

func TestWktreeRemoveForceDeletesUnmergedBranch(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)
	sourceBranch := git(t, []string{"branch", "--show-current"}, repo.sourceRoot)

	git(t, []string{"checkout", "-b", "feature/unmerged"}, repo.sourceRoot)
	write(t, filepath.Join(repo.sourceRoot, "unmerged.txt"), "unmerged\n")
	git(t, []string{"add", "unmerged.txt"}, repo.sourceRoot)
	git(t, []string{"commit", "-m", "Unmerged commit"}, repo.sourceRoot)
	git(t, []string{"checkout", sourceBranch}, repo.sourceRoot)

	result := runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "feature/unmerged"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("switch status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-unmerged")

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
	env := testEnv(t, repo.root)

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

func TestWktreeNewAndRemoveWorkspaces(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	frontendRoot := createSiblingRepo(t, repo.root, "frontend", "frontend")
	env := testEnv(t, repo.root)
	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"worktree_dir: " + repo.worktreeHome,
		"tmux_mode: window",
		"workspace_mode: all",
		"workspaces:",
		"  - name: backend",
		"    panes:",
		"      - command: nvim",
		"  - name: frontend",
		"    repo: ../frontend",
		"    panes:",
		"      - commands:",
		"          - pnpm install",
		"          - pnpm run dev",
		"",
	}, "\n"))

	result := runWktree(t, binary, []string{"new", "feature/full"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("new workspaces status=%d stderr=%s", result.exitCode, result.stderr)
	}
	if strings.Contains(result.stdout, "backend:") || strings.Contains(result.stdout, "frontend:") {
		t.Fatalf("unexpected workspace setup logs without setup actions:\n%s", result.stdout)
	}
	backendPath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-full")
	frontendPath := filepath.Join(repo.worktreeHome, "alienxp03", "frontend", "feature-full")
	for _, item := range []struct {
		path   string
		branch string
	}{
		{backendPath, "feature/full"},
		{frontendPath, "feature/full"},
	} {
		if got := git(t, []string{"branch", "--show-current"}, item.path); got != item.branch {
			t.Fatalf("%s branch = %q", item.path, got)
		}
	}
	backendEnv := read(t, filepath.Join(backendPath, ".wktree.env"))
	for _, want := range []string{"WKTREE_BACKEND_DIR=", "WKTREE_FRONTEND_DIR="} {
		if !strings.Contains(backendEnv, want) {
			t.Fatalf("backend env missing %q:\n%s", want, backendEnv)
		}
	}
	for _, notWant := range []string{"WKTREE_BRANCH=", "WKTREE_WORKSPACE=", "WKTREE_WORKSPACE_DIR="} {
		if strings.Contains(backendEnv, notWant) {
			t.Fatalf("backend env contains noisy var %q:\n%s", notWant, backendEnv)
		}
	}
	if !strings.Contains(backendEnv, frontendPath) {
		t.Fatalf("backend env missing frontend path:\n%s", backendEnv)
	}

	result = runWktree(t, binary, []string{"remove", "--dry-run", "feature/full"}, repo.sourceRoot, env)
	if result.exitCode != 0 || !strings.Contains(result.stdout, "workspace: frontend") || !strings.Contains(result.stdout, "tmux mode: session") || !strings.Contains(result.stdout, "tmux kill-session") {
		t.Fatalf("dry-run status=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}

	result = runWktree(t, binary, []string{"remove", "--force", "feature/full"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("remove workspaces status=%d stderr=%s", result.exitCode, result.stderr)
	}
	for _, want := range []string{
		"remove: backend: cleaning generated workspace env",
		"remove: frontend: cleaning generated workspace env",
		"remove: closing tmux targets",
		"remove: backend: removing git worktree",
		"remove: frontend: deleting local branch",
	} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("remove workspaces output missing %q:\n%s", want, result.stdout)
		}
	}
	if _, err := os.Stat(backendPath); !os.IsNotExist(err) {
		t.Fatalf("expected backend removed, stat err=%v", err)
	}
	if _, err := os.Stat(frontendPath); !os.IsNotExist(err) {
		t.Fatalf("expected frontend removed, stat err=%v", err)
	}
	if got := git(t, []string{"branch", "--list", "feature/full"}, repo.sourceRoot); got != "" {
		t.Fatalf("backend branch still exists: %q", got)
	}
	if got := git(t, []string{"branch", "--list", "feature/full"}, frontendRoot); got != "" {
		t.Fatalf("frontend branch still exists: %q", got)
	}
}

func TestWktreeRemoveRequiresWorkspacesForMultiWorkspaceEnv(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	frontendRoot := createSiblingRepo(t, repo.root, "frontend", "frontend")
	env := testEnv(t, repo.root)
	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"worktree_dir: " + repo.worktreeHome,
		"tmux_mode: window",
		"workspaces:",
		"  - name: backend",
		"  - name: frontend",
		"    repo: ../frontend",
		"",
	}, "\n"))

	result := runWktree(t, binary, []string{"new", "--workspaces", "feature/guard"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("new workspaces status=%d stderr=%s", result.exitCode, result.stderr)
	}
	backendPath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-guard")
	frontendPath := filepath.Join(repo.worktreeHome, "alienxp03", "frontend", "feature-guard")

	result = runWktree(t, binary, []string{"remove", "--force", "feature/guard"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "multiple workspaces") || !strings.Contains(result.stderr, "--workspaces") {
		t.Fatalf("remove status=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
	if _, err := os.Stat(backendPath); err != nil {
		t.Fatalf("expected backend to remain, stat err=%v", err)
	}
	if _, err := os.Stat(frontendPath); err != nil {
		t.Fatalf("expected frontend to remain, stat err=%v", err)
	}

	result = runWktree(t, binary, []string{"close", "feature/guard"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "multiple workspaces") || !strings.Contains(result.stderr, "--workspaces") {
		t.Fatalf("close status=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}

	result = runWktree(t, binary, []string{"close", "--workspaces", "feature/guard"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("close workspaces status=%d stderr=%s", result.exitCode, result.stderr)
	}
	if !strings.Contains(result.stdout, "close: closing tmux targets") || !strings.Contains(result.stdout, "closed feature/guard") {
		t.Fatalf("close workspaces output = %s", result.stdout)
	}
	if _, err := os.Stat(backendPath); err != nil {
		t.Fatalf("expected close to keep backend, stat err=%v", err)
	}
	if _, err := os.Stat(frontendPath); err != nil {
		t.Fatalf("expected close to keep frontend, stat err=%v", err)
	}

	result = runWktree(t, binary, []string{"remove", "--force", "--workspaces", "feature/guard"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("remove workspaces status=%d stderr=%s", result.exitCode, result.stderr)
	}
	if _, err := os.Stat(backendPath); !os.IsNotExist(err) {
		t.Fatalf("expected backend removed, stat err=%v", err)
	}
	if _, err := os.Stat(frontendPath); !os.IsNotExist(err) {
		t.Fatalf("expected frontend removed, stat err=%v", err)
	}
	if got := git(t, []string{"branch", "--list", "feature/guard"}, frontendRoot); got != "" {
		t.Fatalf("frontend branch still exists: %q", got)
	}
}

func TestWktreeWorkspaceRepoSubdirUsesSubdirForSetup(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	repoBRoot := createSiblingRepo(t, repo.root, "repo_b", "repo-b")
	projectARoot := filepath.Join(repoBRoot, "projects", "project_a")
	must(t, os.MkdirAll(projectARoot, 0o755))
	write(t, filepath.Join(projectARoot, ".env"), "SERVER_PORT=3000\n")
	write(t, filepath.Join(projectARoot, "AGENTS.override.md"), "project A agents\n")
	write(t, filepath.Join(projectARoot, "README.md"), "project A\n")
	git(t, []string{"add", "projects/project_a/README.md"}, repoBRoot)
	git(t, []string{"commit", "-m", "Add project A"}, repoBRoot)
	env := testEnv(t, repo.root)
	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"worktree_dir: " + repo.worktreeHome,
		"workspace_mode: all",
		"workspaces:",
		"  - name: repo_a",
		"  - name: project_a",
		"    repo: ../repo_b/projects/project_a",
		"    randomize_ports:",
		"      - file: .env",
		"        vars:",
		"          - SERVER_PORT",
		"    hooks:",
		"      post_create:",
		"        - pwd > cwd.txt",
		"defaults:",
		"  files:",
		"    copy:",
		"      - .env",
		"    symlink:",
		"      - AGENTS.override.md",
		"",
	}, "\n"))

	result := runWktree(t, binary, []string{"new", "feature/subdir"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("new subdir workspace status=%d stderr=%s", result.exitCode, result.stderr)
	}
	for _, want := range []string{"project_a: copied .env", "project_a: symlinked AGENTS.override.md", "project_a: randomized ports in .env", "project_a: $ pwd > cwd.txt"} {
		if !strings.Contains(result.stdout, want) {
			t.Fatalf("setup log missing %q:\n%s", want, result.stdout)
		}
	}
	repoBWorktree := filepath.Join(repo.worktreeHome, "alienxp03", "repo-b", "feature-subdir")
	projectAWorktree := filepath.Join(repoBWorktree, "projects", "project_a")
	if _, err := os.Stat(filepath.Join(projectAWorktree, ".env")); err != nil {
		t.Fatalf("expected project A .env copied into workspace subdir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoBWorktree, ".env")); !os.IsNotExist(err) {
		t.Fatalf("did not expect .env at repo B worktree root, stat err=%v", err)
	}
	assertSymlinkTarget(t, filepath.Join(projectAWorktree, "AGENTS.override.md"), filepath.Join(projectARoot, "AGENTS.override.md"))
	if got := read(t, filepath.Join(projectAWorktree, "cwd.txt")); got != projectAWorktree+"\n" {
		t.Fatalf("hook cwd = %q, want %q", got, projectAWorktree+"\n")
	}
	envFile := read(t, filepath.Join(projectAWorktree, ".wktree.env"))
	if !strings.Contains(envFile, "WKTREE_PROJECT_A_DIR=") || !strings.Contains(envFile, projectAWorktree) {
		t.Fatalf("project A env should point at workspace subdir:\n%s", envFile)
	}
	if got := envFileValue(t, read(t, filepath.Join(projectAWorktree, ".env")), "SERVER_PORT"); !isPort(got) || got == "3000" {
		t.Fatalf("SERVER_PORT should be randomized, got %q", got)
	}
}

func TestWktreeProjectLocalConfigUsesOpenedFolder(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	projectRoot := filepath.Join(repo.sourceRoot, "projects", "project_a")
	must(t, os.MkdirAll(projectRoot, 0o755))
	write(t, filepath.Join(projectRoot, "README.md"), "project A\n")
	write(t, filepath.Join(projectRoot, ".env"), "APP_PORT=3000\n")
	git(t, []string{"add", "projects/project_a/README.md"}, repo.sourceRoot)
	git(t, []string{"commit", "-m", "Add project A"}, repo.sourceRoot)
	write(t, filepath.Join(projectRoot, ".wktree.yaml"), strings.Join([]string{
		"worktree_dir: " + repo.worktreeHome,
		"workspaces:",
		"  - name: project_a",
		"    files:",
		"      copy:",
		"        - .env",
		"    hooks:",
		"      post_create:",
		"        - pwd > cwd.txt",
		"",
	}, "\n"))
	env := testEnv(t, repo.root)

	result := runWktree(t, binary, []string{"new", "feature/project-local"}, projectRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("project local new status=%d stderr=%s", result.exitCode, result.stderr)
	}
	repoWorktree := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-project-local")
	projectWorktree := filepath.Join(repoWorktree, "projects", "project_a")
	if _, err := os.Stat(filepath.Join(projectWorktree, ".env")); err != nil {
		t.Fatalf("expected .env copied into opened project folder: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoWorktree, ".env")); !os.IsNotExist(err) {
		t.Fatalf("did not expect .env at repo worktree root, stat err=%v", err)
	}
	if got := read(t, filepath.Join(projectWorktree, "cwd.txt")); got != projectWorktree+"\n" {
		t.Fatalf("hook cwd = %q, want %q", got, projectWorktree+"\n")
	}
}

func TestWktreeSetEnvUsesRandomizedWorkspaceValues(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	repoBRoot := createSiblingRepo(t, repo.root, "repo_b", "repo-b")
	write(t, filepath.Join(repo.sourceRoot, ".env"), "PORT=3000\n")
	write(t, filepath.Join(repoBRoot, ".env"), "URL=http://localhost:5001/url\n")
	env := testEnv(t, repo.root)
	openLog := filepath.Join(repo.root, "open.log")
	env = append(env, "WKTREE_TEST_OPEN_LOG="+openLog)
	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"worktree_dir: " + repo.worktreeHome,
		"workspace_mode: all",
		"workspaces:",
		"  - name: repo_a",
		"    files:",
		"      copy:",
		"        - .env",
		"    randomize_ports:",
		"      - file: .env",
		"        vars:",
		"          - PORT",
		"  - name: repo_b",
		"    repo: ../repo_b",
		"    files:",
		"      copy:",
		"        - .env",
		"    set_env:",
		"      - file: .env",
		"        vars:",
		"          URL: http://localhost:${repo_a:PORT}/url",
		"    open:",
		"      - http://localhost:${repo_a:PORT}/url",
		"",
	}, "\n"))

	result := runWktree(t, binary, []string{"new", "feature/set-env"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("set_env status=%d stderr=%s", result.exitCode, result.stderr)
	}
	repoAWorktree := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-set-env")
	repoBWorktree := filepath.Join(repo.worktreeHome, "alienxp03", "repo-b", "feature-set-env")
	port := envFileValue(t, read(t, filepath.Join(repoAWorktree, ".env")), "PORT")
	if !isPort(port) || port == "3000" {
		t.Fatalf("PORT should be randomized, got %q", port)
	}
	if got := envFileValue(t, read(t, filepath.Join(repoBWorktree, ".env")), "URL"); got != "http://localhost:"+port+"/url" {
		t.Fatalf("URL = %q, want port %s", got, port)
	}
	if !strings.Contains(result.stdout, "repo_b: set env in .env") {
		t.Fatalf("setup log missing set_env:\n%s", result.stdout)
	}
	if !strings.Contains(result.stdout, "repo_b: opened http://localhost:"+port+"/url") {
		t.Fatalf("setup log missing open:\n%s", result.stdout)
	}
	if got := strings.TrimSpace(read(t, openLog)); got != "http://localhost:"+port+"/url" {
		t.Fatalf("open log = %q, want port %s", got, port)
	}
}

func TestWktreeRemoveWorkspacesPreflightsDirtyTargets(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	frontendRoot := createSiblingRepo(t, repo.root, "frontend", "frontend")
	env := testEnv(t, repo.root)
	write(t, filepath.Join(frontendRoot, ".env.local"), "FRONTEND_ENV=1\n")
	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"worktree_dir: " + repo.worktreeHome,
		"workspace_mode: all",
		"workspaces:",
		"  - name: backend",
		"  - name: frontend",
		"    repo: ../frontend",
		"    files:",
		"      copy:",
		"        - .env.local",
		"",
	}, "\n"))

	result := runWktree(t, binary, []string{"new", "feature/dirty"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("new workspaces status=%d stderr=%s", result.exitCode, result.stderr)
	}
	backendPath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-dirty")
	frontendPath := filepath.Join(repo.worktreeHome, "alienxp03", "frontend", "feature-dirty")

	result = runWktree(t, binary, []string{"remove", "--workspaces", "feature/dirty"}, repo.sourceRoot, env)
	if result.exitCode == 0 || !strings.Contains(result.stderr, "frontend: worktree contains modified or untracked files") || !strings.Contains(result.stderr, "--force") {
		t.Fatalf("remove status=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
	if _, err := os.Stat(backendPath); err != nil {
		t.Fatalf("expected backend to remain, stat err=%v", err)
	}
	if got := git(t, []string{"branch", "--list", "feature/dirty"}, repo.sourceRoot); !strings.Contains(got, "feature/dirty") {
		t.Fatalf("expected backend branch to remain: %q", got)
	}
	if _, err := os.Stat(frontendPath); err != nil {
		t.Fatalf("expected frontend to remain, stat err=%v", err)
	}

	result = runWktree(t, binary, []string{"remove", "--force", "--workspaces", "feature/dirty"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("force remove workspaces status=%d stderr=%s", result.exitCode, result.stderr)
	}
}

func TestWktreeNewAppliesSetupConfig(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)
	write(t, filepath.Join(repo.sourceRoot, ".env.local"), "PROJECT_ENV=1\nPORT=3000\nAPP_PORT=3001\n")
	write(t, filepath.Join(repo.sourceRoot, ".tool-versions"), "go 1.26.1\n")
	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"tmux_mode: window",
		"defaults:",
		"  files:",
		"    copy:",
		"      - .env.local",
		"      - .missing",
		"    symlink:",
		"      - .tool-versions",
		"      - .missing-link",
		"workspaces:",
		"  - name: app",
		"    randomize_ports:",
		"      - file: .env.local",
		"        vars:",
		"          - PORT",
		"          - APP_PORT",
		"    hooks:",
		"      post_create:",
		"        - printf project > project.txt",
		"",
	}, "\n"))

	result := runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "feature/setup"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-setup")
	projectLink := filepath.Join(worktreePath, ".tool-versions")
	envLocal := read(t, filepath.Join(worktreePath, ".env.local"))
	if !strings.Contains(envLocal, "PROJECT_ENV=1\n") {
		t.Fatalf(".env.local = %q", envLocal)
	}
	port := envFileValue(t, envLocal, "PORT")
	appPort := envFileValue(t, envLocal, "APP_PORT")
	if port == "3000" || appPort == "3001" || port == appPort || !isPort(port) || !isPort(appPort) {
		t.Fatalf("ports not randomized as expected: PORT=%q APP_PORT=%q\n%s", port, appPort, envLocal)
	}
	result = runWktree(t, binary, []string{"switch", "--home", repo.worktreeHome, "feature/setup"}, repo.sourceRoot, env)
	if result.exitCode != 0 {
		t.Fatalf("switch status=%d stderr=%s", result.exitCode, result.stderr)
	}
	switchedEnvLocal := read(t, filepath.Join(worktreePath, ".env.local"))
	if switchedEnvLocal != envLocal {
		t.Fatalf("switch should preserve randomized ports:\nbefore:\n%s\nafter:\n%s", envLocal, switchedEnvLocal)
	}
	assertSymlinkTarget(t, projectLink, filepath.Join(repo.sourceRoot, ".tool-versions"))
	if got := read(t, filepath.Join(worktreePath, "project.txt")); got != "project" {
		t.Fatalf("project.txt = %q", got)
	}
	if got := read(t, filepath.Join(worktreePath, ".wktree.env")); !strings.Contains(got, "WKTREE_APP_DIR") {
		t.Fatalf("workspace env = %q", got)
	}
	if !strings.Contains(result.stderr, "copy source not found, skipping: .missing") {
		t.Fatalf("stderr missing copy warning: %s", result.stderr)
	}
	if !strings.Contains(result.stderr, "symlink source not found, skipping: .missing-link") {
		t.Fatalf("stderr missing symlink warning: %s", result.stderr)
	}
}

func TestFailingPostCreateLeavesWorktreeIntact(t *testing.T) {
	binary := buildBinary(t)
	repo := createTempRepo(t)
	env := testEnv(t, repo.root)
	write(t, filepath.Join(repo.sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"tmux_mode: window",
		"workspaces:",
		"  - name: app",
		"    hooks:",
		"      post_create:",
		"        - exit 1",
		"",
	}, "\n"))

	result := runWktree(t, binary, []string{"new", "--home", repo.worktreeHome, "feature/failsetup"}, repo.sourceRoot, env)
	if result.exitCode != 1 || !strings.Contains(result.stderr, "post create command failed") {
		t.Fatalf("expected setup failure, status=%d stderr=%s", result.exitCode, result.stderr)
	}
	worktreePath := filepath.Join(repo.worktreeHome, "alienxp03", "demo", "feature-failsetup")
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

func createSiblingRepo(t *testing.T, root string, dirName string, remoteRepo string) string {
	t.Helper()
	sourceRoot := filepath.Join(root, dirName)
	must(t, os.MkdirAll(sourceRoot, 0o755))
	git(t, []string{"init"}, sourceRoot)
	git(t, []string{"config", "user.email", "test@example.com"}, sourceRoot)
	git(t, []string{"config", "user.name", "Test User"}, sourceRoot)
	write(t, filepath.Join(sourceRoot, "README.md"), remoteRepo+"\n")
	git(t, []string{"add", "README.md"}, sourceRoot)
	git(t, []string{"commit", "-m", "Initial commit"}, sourceRoot)
	git(t, []string{"remote", "add", "origin", "git@github.com:alienxp03/" + remoteRepo + ".git"}, sourceRoot)
	return sourceRoot
}

func enableLocalOrigin(t *testing.T, repo tempRepo) string {
	t.Helper()
	originRoot := filepath.Join(repo.root, "origin.git")
	git(t, []string{"init", "--bare", originRoot}, repo.root)
	git(t, []string{"config", "url." + originRoot + ".insteadOf", "git@github.com:alienxp03/demo.git"}, repo.sourceRoot)
	return originRoot
}

func pushPullRefCommit(t *testing.T, repo tempRepo, number int, relativePath string, content string) string {
	t.Helper()
	write(t, filepath.Join(repo.sourceRoot, relativePath), content)
	git(t, []string{"add", relativePath}, repo.sourceRoot)
	git(t, []string{"commit", "-m", fmt.Sprintf("PR %d update", number)}, repo.sourceRoot)
	head := git(t, []string{"rev-parse", "HEAD"}, repo.sourceRoot)
	git(t, []string{"push", "origin", "HEAD:refs/pull/" + strconv.Itoa(number) + "/head"}, repo.sourceRoot)
	return head
}

func writeFakeGh(t *testing.T, env []string, response string) {
	t.Helper()
	pathValue := envValue(env, "PATH")
	binDir := strings.Split(pathValue, string(os.PathListSeparator))[0]
	must(t, os.WriteFile(filepath.Join(binDir, "gh"), []byte(strings.Join([]string{
		"#!/bin/sh",
		"if [ \"$1\" = \"pr\" ] && [ \"$2\" = \"view\" ]; then",
		"  cat <<'JSON'",
		response,
		"JSON",
		"  exit 0",
		"fi",
		"echo unexpected gh args: \"$@\" >&2",
		"exit 1",
		"",
	}, "\n")), 0o755))
}

func pullRequestJSON(number int, headRefName string, headRefOID string, url string) string {
	return fmt.Sprintf(`{"number":%d,"headRefName":%q,"headRefOid":%q,"url":%q}`, number, headRefName, headRefOID, url)
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

func testEnv(t *testing.T, root string) []string {
	t.Helper()
	binDir := filepath.Join(root, "bin")
	must(t, os.MkdirAll(binDir, 0o755))
	must(t, os.WriteFile(filepath.Join(binDir, "tmux"), []byte(strings.Join([]string{
		"#!/bin/sh",
		"case \"$1\" in",
		"  -V) echo 'tmux 3.5'; exit 0 ;;",
		"  has-session) exit 1 ;;",
		"  list-windows) exit 0 ;;",
		"  new-session|new-window|split-window) echo '%9'; exit 0 ;;",
		"  *) exit 0 ;;",
		"esac",
		"",
	}, "\n")), 0o755))
	openScript := strings.Join([]string{
		"#!/bin/sh",
		"if [ -n \"${WKTREE_TEST_OPEN_LOG:-}\" ]; then",
		"  printf '%s\\n' \"$1\" >> \"$WKTREE_TEST_OPEN_LOG\"",
		"fi",
		"exit 0",
		"",
	}, "\n")
	for _, name := range []string{"open", "xdg-open"} {
		must(t, os.WriteFile(filepath.Join(binDir, name), []byte(openScript), 0o755))
	}
	return append(os.Environ(),
		"HOME="+filepath.Join(root, "home"),
		"XDG_CONFIG_HOME="+filepath.Join(root, "xdg"),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMUX=/tmp/tmux",
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

func envFileValue(t *testing.T, content string, key string) string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	t.Fatalf("env file missing %s:\n%s", key, content)
	return ""
}

func isPort(value string) bool {
	port, err := strconv.Atoi(value)
	return err == nil && port > 0 && port <= 65535
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
