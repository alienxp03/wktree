package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alienxp03/wktree/internal/paths"
	"github.com/alienxp03/wktree/internal/run"
)

type Worktree struct {
	Branch       string
	BranchSlug   string
	RepoRoot     string
	RepoSlug     string
	Reused       bool
	WorktreeHome string
	WorktreePath string
}

type WorktreeList struct {
	CurrentPath string
	Worktrees   []ListedWorktree
}

type ListedWorktree struct {
	Path     string
	Head     string
	Branch   string
	Detached bool
}

type RemoveOptions struct {
	Branch string
	Cwd    string
	Force  bool
	Runner run.Runner
}

type RemoveTarget struct {
	Branch       string
	RepoRoot     string
	WorktreePath string
}

type CreateOptions struct {
	Branch     string
	From       string
	HomeOption string
	Cwd        string
	Runner     run.Runner
}

type SwitchTarget struct {
	Worktree   Worktree
	StartPoint string
	Track      bool
}

type CreateTarget struct {
	Worktree   Worktree
	StartPoint string
}

func RepoRoot(ctx context.Context, cwd string, runner run.Runner) (string, error) {
	result := runGit(ctx, runner, cwd, []string{"rev-parse", "--show-toplevel"}, false)
	if result.Err != nil || result.ExitCode != 0 {
		return "", errors.New(run.FailureMessage("git", []string{"rev-parse", "--show-toplevel"}, result))
	}
	return strings.TrimSpace(result.Stdout), nil
}

func OriginRemoteURL(ctx context.Context, repoRoot string, runner run.Runner) string {
	result := runGit(ctx, runner, repoRoot, []string{"config", "--get", "remote.origin.url"}, true)
	if result.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

func UserName(ctx context.Context, repoRoot string, runner run.Runner) string {
	result := runGit(ctx, runner, repoRoot, []string{"config", "--get", "user.name"}, true)
	if result.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

func RepoSlug(ctx context.Context, repoRoot string, runner run.Runner) (string, error) {
	remote := OriginRemoteURL(ctx, repoRoot, runner)
	if slug, ok, err := paths.ParseGitHubRemote(remote); ok || err != nil {
		return slug, err
	}
	userName := UserName(ctx, repoRoot, runner)
	if userName == "" {
		return "", fmt.Errorf("repo has no supported GitHub remote and git config user.name is not set")
	}
	return paths.RepoDirectorySlug(repoRoot, userName)
}

func ListWorktrees(ctx context.Context, cwd string, runner run.Runner) (WorktreeList, error) {
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	repoRoot, err := RepoRoot(ctx, cwd, runner)
	if err != nil {
		return WorktreeList{}, err
	}
	worktrees, err := listWorktrees(ctx, repoRoot, runner)
	if err != nil {
		return WorktreeList{}, err
	}
	return WorktreeList{CurrentPath: repoRoot, Worktrees: worktrees}, nil
}

func CompleteSwitchBranches(ctx context.Context, cwd string, prefix string, runner run.Runner) ([]string, error) {
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	repoRoot, err := RepoRoot(ctx, cwd, runner)
	if err != nil {
		return nil, err
	}
	names, err := branchNames(ctx, repoRoot, runner)
	if err != nil {
		return nil, err
	}
	return filterPrefix(names, paths.StripOriginPrefix(prefix)), nil
}

func CompleteRemoveBranches(ctx context.Context, cwd string, prefix string, runner run.Runner) ([]string, error) {
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	repoRoot, err := RepoRoot(ctx, cwd, runner)
	if err != nil {
		return nil, err
	}
	worktrees, err := listWorktrees(ctx, repoRoot, runner)
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, worktree := range worktrees {
		if worktree.Detached || worktree.Branch == "" || samePath(repoRoot, worktree.Path) {
			continue
		}
		names = append(names, worktree.Branch)
	}
	return filterPrefix(uniqueSorted(names), paths.StripOriginPrefix(prefix)), nil
}

func CreateWorktree(ctx context.Context, options CreateOptions) (Worktree, error) {
	target, err := ResolveCreateWorktree(ctx, options)
	if err != nil {
		return Worktree{}, err
	}
	return CreateResolvedWorktree(ctx, target, options.Runner)
}

func ResolveCreateWorktree(ctx context.Context, options CreateOptions) (CreateTarget, error) {
	runner := options.Runner
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	if options.Branch == "" {
		return CreateTarget{}, fmt.Errorf("branch name is required")
	}

	repoRoot, err := RepoRoot(ctx, options.Cwd, runner)
	if err != nil {
		return CreateTarget{}, err
	}
	if err := ensureBranchName(ctx, repoRoot, options.Branch, runner); err != nil {
		return CreateTarget{}, err
	}
	startPoint := options.From
	if startPoint == "" {
		startPoint = "HEAD"
	}
	if err := ensureStartPointExists(ctx, repoRoot, startPoint, runner); err != nil {
		return CreateTarget{}, err
	}
	if exists, err := refExists(ctx, repoRoot, "refs/heads/"+options.Branch, runner); err != nil {
		return CreateTarget{}, err
	} else if exists {
		return CreateTarget{}, fmt.Errorf("local branch already exists: %s", options.Branch)
	}

	remoteBranch := paths.StripOriginPrefix(options.Branch)
	if exists, err := refExists(ctx, repoRoot, "refs/remotes/origin/"+remoteBranch, runner); err != nil {
		return CreateTarget{}, err
	} else if exists {
		return CreateTarget{}, fmt.Errorf("origin branch already exists: origin/%s", remoteBranch)
	}

	worktree, err := buildWorktree(ctx, repoRoot, options.Branch, options.HomeOption, runner)
	if err != nil {
		return CreateTarget{}, err
	}
	if err := ensureTargetPathAvailable(worktree.WorktreePath); err != nil {
		return CreateTarget{}, err
	}

	return CreateTarget{Worktree: worktree, StartPoint: startPoint}, nil
}

func CreateResolvedWorktree(ctx context.Context, target CreateTarget, runner run.Runner) (Worktree, error) {
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	if err := ensureTargetPathAvailable(target.Worktree.WorktreePath); err != nil {
		return Worktree{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target.Worktree.WorktreePath), 0o755); err != nil {
		return Worktree{}, err
	}
	addArgs := []string{"worktree", "add", "-b", target.Worktree.Branch, target.Worktree.WorktreePath, target.StartPoint}
	result := runGit(ctx, runner, target.Worktree.RepoRoot, addArgs, false)
	if result.Err != nil || result.ExitCode != 0 {
		return Worktree{}, errors.New(run.FailureMessage("git", addArgs, result))
	}

	return target.Worktree, nil
}

func ResolveSwitchWorktree(ctx context.Context, options CreateOptions) (SwitchTarget, error) {
	runner := options.Runner
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	branch := paths.StripOriginPrefix(options.Branch)
	if branch == "" {
		return SwitchTarget{}, fmt.Errorf("branch name is required")
	}

	repoRoot, err := RepoRoot(ctx, options.Cwd, runner)
	if err != nil {
		return SwitchTarget{}, err
	}
	if err := ensureBranchName(ctx, repoRoot, branch, runner); err != nil {
		return SwitchTarget{}, err
	}
	if err := ensureHeadExists(ctx, repoRoot, runner); err != nil {
		return SwitchTarget{}, err
	}

	worktree, err := buildWorktree(ctx, repoRoot, branch, options.HomeOption, runner)
	if err != nil {
		return SwitchTarget{}, err
	}

	if exists, err := refExists(ctx, repoRoot, "refs/heads/"+branch, runner); err != nil {
		return SwitchTarget{}, err
	} else if exists {
		if worktreePath, ok, err := checkedOutWorktree(ctx, repoRoot, branch, runner); err != nil {
			return SwitchTarget{}, err
		} else if ok {
			worktree.Reused = true
			worktree.WorktreePath = worktreePath
			return SwitchTarget{Worktree: worktree}, nil
		}
		if err := ensureTargetPathAvailable(worktree.WorktreePath); err != nil {
			return SwitchTarget{}, err
		}
		return SwitchTarget{Worktree: worktree, StartPoint: branch}, nil
	}

	remote := "origin/" + branch
	if exists, err := refExists(ctx, repoRoot, "refs/remotes/"+remote, runner); err != nil {
		return SwitchTarget{}, err
	} else if exists {
		if err := ensureTargetPathAvailable(worktree.WorktreePath); err != nil {
			return SwitchTarget{}, err
		}
		return SwitchTarget{Worktree: worktree, StartPoint: remote, Track: true}, nil
	}

	return SwitchTarget{}, fmt.Errorf("branch does not exist locally or on origin: %s (use wktree new %s to create it)", branch, branch)
}

func SwitchWorktree(ctx context.Context, target SwitchTarget, runner run.Runner) (Worktree, error) {
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	if target.Worktree.Reused {
		return target.Worktree, nil
	}
	if err := ensureTargetPathAvailable(target.Worktree.WorktreePath); err != nil {
		return Worktree{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target.Worktree.WorktreePath), 0o755); err != nil {
		return Worktree{}, err
	}

	args := []string{"worktree", "add"}
	if target.Track {
		args = append(args, "--track", "-b", target.Worktree.Branch)
	}
	args = append(args, target.Worktree.WorktreePath, target.StartPoint)

	result := runGit(ctx, runner, target.Worktree.RepoRoot, args, false)
	if result.Err != nil || result.ExitCode != 0 {
		return Worktree{}, errors.New(run.FailureMessage("git", args, result))
	}
	return target.Worktree, nil
}

func ResolveRemoveTarget(ctx context.Context, options RemoveOptions) (RemoveTarget, error) {
	runner := options.Runner
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	branch := paths.StripOriginPrefix(options.Branch)
	if branch == "" {
		return RemoveTarget{}, fmt.Errorf("branch name is required")
	}

	repoRoot, err := RepoRoot(ctx, options.Cwd, runner)
	if err != nil {
		return RemoveTarget{}, err
	}
	if err := ensureBranchName(ctx, repoRoot, branch, runner); err != nil {
		return RemoveTarget{}, err
	}
	if exists, err := refExists(ctx, repoRoot, "refs/heads/"+branch, runner); err != nil {
		return RemoveTarget{}, err
	} else if !exists {
		return RemoveTarget{}, fmt.Errorf("local branch does not exist: %s", branch)
	}

	worktreePath, ok, err := checkedOutWorktree(ctx, repoRoot, branch, runner)
	if err != nil {
		return RemoveTarget{}, err
	}
	if !ok {
		return RemoveTarget{}, fmt.Errorf("branch has no worktree: %s", branch)
	}
	if samePath(repoRoot, worktreePath) {
		return RemoveTarget{}, fmt.Errorf("cannot remove current worktree: %s", branch)
	}
	if !options.Force {
		if err := ensureBranchMerged(ctx, repoRoot, branch, runner); err != nil {
			return RemoveTarget{}, err
		}
	}
	return RemoveTarget{Branch: branch, RepoRoot: repoRoot, WorktreePath: worktreePath}, nil
}

func RemoveWorktree(ctx context.Context, target RemoveTarget, force bool, runner run.Runner) error {
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	removeArgs := []string{"worktree", "remove"}
	if force {
		removeArgs = append(removeArgs, "--force")
	}
	removeArgs = append(removeArgs, target.WorktreePath)
	removed := runGit(ctx, runner, target.RepoRoot, removeArgs, false)
	if removed.Err != nil || removed.ExitCode != 0 {
		return errors.New(run.FailureMessage("git", removeArgs, removed))
	}
	deleteArgs := []string{"branch", "-d"}
	if force {
		deleteArgs = []string{"branch", "-D"}
	}
	deleteArgs = append(deleteArgs, target.Branch)
	deleted := runGit(ctx, runner, target.RepoRoot, deleteArgs, false)
	if deleted.Err != nil || deleted.ExitCode != 0 {
		return errors.New(run.FailureMessage("git", deleteArgs, deleted))
	}
	return nil
}

func EnsureCleanWorktree(ctx context.Context, target RemoveTarget, runner run.Runner) error {
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	args := []string{"status", "--porcelain"}
	result := runGit(ctx, runner, target.WorktreePath, args, false)
	if result.Err != nil || result.ExitCode != 0 {
		return errors.New(run.FailureMessage("git", args, result))
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		if line == "" || isGeneratedStatusLine(line) {
			continue
		}
		return fmt.Errorf("worktree contains modified or untracked files, use --force to delete it: %s", target.WorktreePath)
	}
	return nil
}

func buildWorktree(ctx context.Context, repoRoot string, branch string, homeOption string, runner run.Runner) (Worktree, error) {
	repoSlug, err := RepoSlug(ctx, repoRoot, runner)
	if err != nil {
		return Worktree{}, err
	}
	branchSlug, err := paths.BranchSlug(branch)
	if err != nil {
		return Worktree{}, err
	}
	worktreeHome, err := paths.WorktreeHome(homeOption)
	if err != nil {
		return Worktree{}, err
	}
	return Worktree{
		Branch:       branch,
		BranchSlug:   branchSlug,
		RepoRoot:     repoRoot,
		RepoSlug:     repoSlug,
		WorktreeHome: worktreeHome,
		WorktreePath: paths.WorktreePath(worktreeHome, repoSlug, branchSlug),
	}, nil
}

func ensureTargetPathAvailable(worktreePath string) error {
	if _, err := os.Lstat(worktreePath); err == nil {
		return fmt.Errorf("target worktree path already exists: %s", worktreePath)
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func checkedOutWorktree(ctx context.Context, repoRoot string, branch string, runner run.Runner) (string, bool, error) {
	worktrees, err := listWorktrees(ctx, repoRoot, runner)
	if err != nil {
		return "", false, err
	}
	for _, worktree := range worktrees {
		if worktree.Branch == branch {
			return worktree.Path, true, nil
		}
	}
	return "", false, nil
}

func listWorktrees(ctx context.Context, repoRoot string, runner run.Runner) ([]ListedWorktree, error) {
	args := []string{"worktree", "list", "--porcelain", "-z"}
	result := runGit(ctx, runner, repoRoot, args, false)
	if result.Err != nil || result.ExitCode != 0 {
		return nil, errors.New(run.FailureMessage("git", args, result))
	}
	return parseWorktreeList(result.Stdout), nil
}

func parseWorktreeList(output string) []ListedWorktree {
	worktrees := []ListedWorktree{}
	current := ListedWorktree{}
	hasCurrent := false
	flush := func() {
		if hasCurrent {
			worktrees = append(worktrees, current)
			current = ListedWorktree{}
			hasCurrent = false
		}
	}
	for _, token := range strings.Split(output, "\x00") {
		switch {
		case token == "":
			flush()
		case strings.HasPrefix(token, "worktree "):
			flush()
			current = ListedWorktree{Path: strings.TrimPrefix(token, "worktree ")}
			hasCurrent = true
		case strings.HasPrefix(token, "HEAD ") && hasCurrent:
			current.Head = strings.TrimPrefix(token, "HEAD ")
		case strings.HasPrefix(token, "branch ") && hasCurrent:
			current.Branch = cleanBranchRef(strings.TrimPrefix(token, "branch "))
		case token == "detached" && hasCurrent:
			current.Detached = true
		}
	}
	flush()
	return worktrees
}

func cleanBranchRef(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

func branchNames(ctx context.Context, repoRoot string, runner run.Runner) ([]string, error) {
	args := []string{"for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes/origin"}
	result := runGit(ctx, runner, repoRoot, args, false)
	if result.Err != nil || result.ExitCode != 0 {
		return nil, errors.New(run.FailureMessage("git", args, result))
	}

	names := []string{}
	for _, line := range strings.Split(result.Stdout, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || name == "origin/HEAD" {
			continue
		}
		names = append(names, paths.StripOriginPrefix(name))
	}
	return uniqueSorted(names), nil
}

func filterPrefix(values []string, prefix string) []string {
	filtered := []string{}
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func uniqueSorted(values []string) []string {
	sort.Strings(values)
	unique := values[:0]
	var previous string
	for index, value := range values {
		if index == 0 || value != previous {
			unique = append(unique, value)
			previous = value
		}
	}
	return unique
}

func ensureBranchMerged(ctx context.Context, repoRoot string, branch string, runner run.Runner) error {
	args := []string{"merge-base", "--is-ancestor", branch, "HEAD"}
	result := runGit(ctx, runner, repoRoot, args, true)
	switch result.ExitCode {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("branch is not merged into current HEAD: %s", branch)
	default:
		return errors.New(run.FailureMessage("git", args, result))
	}
}

func isGeneratedStatusLine(line string) bool {
	if len(line) < 4 {
		return false
	}
	return strings.TrimSpace(line[3:]) == ".wktree.env"
}

func samePath(left string, right string) bool {
	leftAbs := cleanAbsPath(left)
	rightAbs := cleanAbsPath(right)
	if leftAbs == rightAbs {
		return true
	}
	leftEval := evalPath(leftAbs)
	rightEval := evalPath(rightAbs)
	return leftEval != "" && leftEval == rightEval
}

func cleanAbsPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}

func evalPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	return filepath.Clean(resolved)
}

func ensureBranchName(ctx context.Context, repoRoot string, branch string, runner run.Runner) error {
	result := runGit(ctx, runner, repoRoot, []string{"check-ref-format", "--branch", branch}, true)
	if result.ExitCode != 0 {
		return fmt.Errorf("invalid branch name: %s", branch)
	}
	return nil
}

func ensureHeadExists(ctx context.Context, repoRoot string, runner run.Runner) error {
	return ensureStartPointExists(ctx, repoRoot, "HEAD", runner)
}

func ensureStartPointExists(ctx context.Context, repoRoot string, startPoint string, runner run.Runner) error {
	args := []string{"rev-parse", "--verify", startPoint + "^{commit}"}
	result := runGit(ctx, runner, repoRoot, args, false)
	if result.Err != nil || result.ExitCode != 0 {
		return errors.New(run.FailureMessage("git", args, result))
	}
	return nil
}

func refExists(ctx context.Context, repoRoot string, ref string, runner run.Runner) (bool, error) {
	args := []string{"show-ref", "--verify", "--quiet", ref}
	result := runGit(ctx, runner, repoRoot, args, true)
	switch result.ExitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, errors.New(run.FailureMessage("git", args, result))
	}
}

func runGit(ctx context.Context, runner run.Runner, cwd string, args []string, allowFailure bool) run.Result {
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	result := runner.Run(ctx, "git", args, run.Options{Cwd: cwd})
	if !allowFailure && result.ExitCode != 0 && result.Err == nil {
		result.Err = fmt.Errorf("exit code %d", result.ExitCode)
	}
	return result
}
