package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alienxp03/wktree/internal/config"
	"github.com/alienxp03/wktree/internal/git"
	"github.com/alienxp03/wktree/internal/paths"
	"github.com/alienxp03/wktree/internal/run"
	"github.com/alienxp03/wktree/internal/setup"
	"github.com/alienxp03/wktree/internal/shellinit"
	"github.com/alienxp03/wktree/internal/tmux"
)

const Version = "0.1.0"

type Options struct {
	Cwd         string
	Env         map[string]string
	Stdout      io.Writer
	Stderr      io.Writer
	Runner      run.Runner
	ShellRunner setup.ShellRunner
}

type workspaceSpec struct {
	Name             string
	RepoRoot         string
	WorkspaceRoot    string
	WorkspaceRelPath string
	Config           config.Workspace
}

type workspaceSelection struct {
	Config         config.Config
	ConfigPath     string
	ConfigDir      string
	ConfigRepoRoot string
	ConfigRepoSlug string
	WorktreeHome   string
	Workspaces     []workspaceSpec
	AllWorkspaces  bool
}

type workspaceWorktree struct {
	Spec     workspaceSpec
	Worktree git.Worktree
}

type workspacePlan struct {
	plan   setup.Plan
	logger setup.Logger
	open   []string
}

func Run(args []string, options Options) int {
	app := normalizeOptions(options)
	ctx := context.Background()

	if len(args) == 0 || contains(args, "--help") || contains(args, "-h") {
		fmt.Fprint(app.Stdout, HelpText())
		return 0
	}
	if args[0] == "--version" || args[0] == "-v" {
		fmt.Fprintf(app.Stdout, "%s\n", Version)
		return 0
	}

	var err error
	var exitCode int
	switch args[0] {
	case "__complete":
		exitCode, err = runComplete(ctx, args[1:], app)
	case "doctor":
		exitCode, err = runDoctor(ctx, args[1:], app)
	case "list":
		exitCode, err = runList(ctx, args[1:], app)
	case "new":
		exitCode, err = runNew(ctx, args[1:], app)
	case "close":
		exitCode, err = runClose(ctx, args[1:], app)
	case "remove":
		exitCode, err = runRemove(ctx, args[1:], app)
	case "switch":
		exitCode, err = runSwitch(ctx, args[1:], app)
	case "init":
		exitCode, err = runInit(ctx, args[1:], app)
	case "completion":
		exitCode, err = runCompletion(args[1:], app)
	default:
		err = fmt.Errorf("unknown command: %s", args[0])
		exitCode = 1
	}
	if err != nil {
		fmt.Fprintf(app.Stderr, "wktree: %s\n", err)
		return 1
	}
	return exitCode
}

func runComplete(ctx context.Context, args []string, options Options) (int, error) {
	if len(args) < 1 || len(args) > 2 {
		return 1, fmt.Errorf("usage: wktree __complete <command> [prefix]")
	}
	command := args[0]
	prefix := ""
	if len(args) == 2 {
		prefix = args[1]
	}

	var values []string
	var err error
	switch command {
	case "completion":
		values = filterPrefix([]string{"bash", "zsh"}, prefix)
	case "init":
		values = nil
	case "doctor", "list":
		values = nil
	case "new":
		values, err = completeWorktreeCommand(ctx, options, prefix, []string{"--home", "--from", "--workspaces"})
	case "close":
		if strings.HasPrefix(prefix, "-") {
			values = filterPrefix([]string{"--dry-run", "--workspaces"}, prefix)
		} else {
			values, err = git.CompleteCloseBranches(ctx, options.Cwd, prefix, options.Runner)
		}
	case "remove":
		if strings.HasPrefix(prefix, "-") {
			values = filterPrefix([]string{"--force", "--dry-run", "--workspaces"}, prefix)
		} else {
			values, err = git.CompleteRemoveBranches(ctx, options.Cwd, prefix, options.Runner)
		}
	case "switch":
		values, err = completeWorktreeCommand(ctx, options, prefix, []string{"--home", "--workspaces", "--pr"})
	default:
		values = filterPrefix([]string{"doctor", "list", "new", "close", "remove", "switch", "init", "completion"}, prefix)
	}
	if err != nil {
		return 1, err
	}
	for _, value := range values {
		fmt.Fprintln(options.Stdout, value)
	}
	return 0, nil
}

func completeWorktreeCommand(ctx context.Context, options Options, prefix string, flags []string) ([]string, error) {
	if strings.HasPrefix(prefix, "-") {
		return filterPrefix(flags, prefix), nil
	}
	return git.CompleteSwitchBranches(ctx, options.Cwd, prefix, options.Runner)
}

func runList(ctx context.Context, args []string, options Options) (int, error) {
	if len(args) != 0 {
		return 1, fmt.Errorf("usage: wktree list")
	}
	worktreeList, err := git.ListWorktrees(ctx, options.Cwd, options.Runner)
	if err != nil {
		return 1, err
	}
	fmt.Fprint(options.Stdout, renderWorktreeList(worktreeList))
	return 0, nil
}

func runDoctor(ctx context.Context, args []string, options Options) (int, error) {
	if len(args) != 0 {
		return 1, fmt.Errorf("usage: wktree doctor")
	}

	status := 0
	repoRoot, err := git.RepoRoot(ctx, options.Cwd, options.Runner)
	if err != nil {
		fmt.Fprintf(options.Stdout, "[error] git repository: %s\n", err)
		return 1, nil
	}
	fmt.Fprintf(options.Stdout, "[ok] repo root: %s\n", repoRoot)

	if repoSlug, err := git.RepoSlug(ctx, repoRoot, options.Runner); err != nil {
		fmt.Fprintf(options.Stdout, "[error] repo slug: %s\n", err)
		status = 1
	} else {
		fmt.Fprintf(options.Stdout, "[ok] repo slug: %s\n", repoSlug)
	}

	if worktrees, err := git.ListWorktrees(ctx, repoRoot, options.Runner); err != nil {
		fmt.Fprintf(options.Stdout, "[error] worktrees: %s\n", err)
		status = 1
	} else {
		fmt.Fprintf(options.Stdout, "[ok] worktrees: %d\n", len(worktrees.Worktrees))
	}

	selection, err := resolveWorkspaceSelection(ctx, options, "", true)
	if err != nil {
		fmt.Fprintf(options.Stdout, "[error] config: %s\n", err)
		status = 1
	} else {
		fmt.Fprintf(options.Stdout, "[ok] project config path: %s\n", selection.ConfigPath)
		fmt.Fprintf(options.Stdout, "[ok] config: tmux_mode=%s workspace_mode=%s workspaces=%d\n", selection.Config.TmuxMode, selection.Config.WorkspaceMode, len(selection.Workspaces))
	}

	if err := tmux.Available(ctx, options.Runner); err != nil {
		fmt.Fprintf(options.Stdout, "[warn] tmux: not available (%s)\n", err)
	} else {
		fmt.Fprintln(options.Stdout, "[ok] tmux: available")
	}
	fmt.Fprintln(options.Stdout, "[info] shell integration: eval \"$(wktree completion zsh)\" or eval \"$(wktree completion bash)\"")

	return status, nil
}

func runNew(ctx context.Context, args []string, options Options) (int, error) {
	parsed, err := parseWorktreeArgs("new", args)
	if err != nil {
		return 1, err
	}
	selection, err := resolveWorkspaceSelection(ctx, options, parsed.Home, parsed.Workspaces)
	if err != nil {
		return 1, err
	}

	targets := make([]git.CreateTarget, 0, len(selection.Workspaces))
	for _, workspace := range selection.Workspaces {
		target, err := git.ResolveCreateWorktree(ctx, git.CreateOptions{
			Branch:     parsed.Branch,
			From:       parsed.From,
			HomeOption: selection.WorktreeHome,
			Cwd:        workspace.RepoRoot,
			Runner:     options.Runner,
		})
		if err != nil {
			return 1, fmt.Errorf("%s: %w", workspace.Name, err)
		}
		targets = append(targets, target)
	}

	worktrees := make([]workspaceWorktree, 0, len(targets))
	for index, target := range targets {
		worktree, err := git.CreateResolvedWorktree(ctx, target, options.Runner)
		if err != nil {
			return 1, fmt.Errorf("%s: %w", selection.Workspaces[index].Name, err)
		}
		worktrees = append(worktrees, workspaceWorktree{Spec: selection.Workspaces[index], Worktree: worktree})
	}
	return finishWorkspaceCommand(ctx, selection, parsed.Branch, worktrees, options)
}

func runSwitch(ctx context.Context, args []string, options Options) (int, error) {
	parsed, err := parseWorktreeArgs("switch", args)
	if err != nil {
		return 1, err
	}
	if parsed.PullRequest != "" {
		return runSwitchPullRequest(ctx, parsed, options)
	}
	selection, err := resolveWorkspaceSelection(ctx, options, parsed.Home, parsed.Workspaces)
	if err != nil {
		return 1, err
	}

	targets := make([]git.SwitchTarget, 0, len(selection.Workspaces))
	for _, workspace := range selection.Workspaces {
		target, err := git.ResolveSwitchWorktree(ctx, git.CreateOptions{
			Branch:     parsed.Branch,
			HomeOption: selection.WorktreeHome,
			Cwd:        workspace.RepoRoot,
			Runner:     options.Runner,
		})
		if err != nil {
			return 1, fmt.Errorf("%s: %w", workspace.Name, err)
		}
		targets = append(targets, target)
	}

	worktrees := make([]workspaceWorktree, 0, len(targets))
	for index, target := range targets {
		worktree, err := git.SwitchWorktree(ctx, target, options.Runner)
		if err != nil {
			return 1, fmt.Errorf("%s: %w", selection.Workspaces[index].Name, err)
		}
		worktrees = append(worktrees, workspaceWorktree{Spec: selection.Workspaces[index], Worktree: worktree})
	}
	return finishWorkspaceCommand(ctx, selection, parsed.Branch, worktrees, options)
}

func runSwitchPullRequest(ctx context.Context, parsed worktreeArgs, options Options) (int, error) {
	selection, err := resolvePullRequestWorkspaceSelection(ctx, options, parsed.Home)
	if err != nil {
		return 1, err
	}
	target, err := git.ResolvePullRequestWorktree(ctx, git.PullRequestOptions{
		Value:      parsed.PullRequest,
		HomeOption: selection.WorktreeHome,
		Cwd:        selection.Workspaces[0].RepoRoot,
		Runner:     options.Runner,
	})
	if err != nil {
		return 1, fmt.Errorf("%s: %w", selection.Workspaces[0].Name, err)
	}
	if err := ensurePullRequestTargetManaged(selection.Workspaces[0], target); err != nil {
		return 1, fmt.Errorf("%s: %w", selection.Workspaces[0].Name, err)
	}
	worktree, err := git.OpenPullRequestWorktree(ctx, target, options.Runner)
	if err != nil {
		return 1, fmt.Errorf("%s: %w", selection.Workspaces[0].Name, err)
	}

	prContext := &setup.PullRequestContext{
		Number:  target.PullRequest.Number,
		URL:     target.PullRequest.URL,
		HeadRef: target.PullRequest.HeadRefName,
		HeadSHA: target.PullRequest.HeadRefOID,
	}
	worktrees := []workspaceWorktree{{Spec: selection.Workspaces[0], Worktree: worktree}}
	return finishWorkspaceCommandWithContext(ctx, selection, worktree.Branch, worktrees, prContext, options)
}

func runRemove(ctx context.Context, args []string, options Options) (int, error) {
	parsed, err := parseRemoveArgs(args)
	if err != nil {
		return 1, err
	}
	selection, err := resolveWorkspaceSelection(ctx, options, "", parsed.Workspaces)
	if err != nil {
		return 1, err
	}

	targets := make([]git.RemoveTarget, 0, len(selection.Workspaces))
	for _, workspace := range selection.Workspaces {
		target, err := git.ResolveRemoveTarget(ctx, git.RemoveOptions{Branch: parsed.Branch, Cwd: workspace.RepoRoot, Force: parsed.Force, Runner: options.Runner})
		if err != nil {
			return 1, fmt.Errorf("%s: %w", workspace.Name, err)
		}
		targets = append(targets, target)
	}
	if !selection.AllWorkspaces {
		if err := ensureSingleWorkspaceLayout(parsed.Branch, "remove", selection.Workspaces[0], targets[0]); err != nil {
			return 1, err
		}
	}
	if parsed.DryRun {
		fmt.Fprint(options.Stdout, renderRemoveDryRun(selection, parsed.Branch, targets, parsed.Force))
		return 0, nil
	}
	if !parsed.Force {
		for index, target := range targets {
			logRemoveProgress(options.Stdout, selection.Workspaces[index], "checking clean worktree")
			if err := git.EnsureCleanWorktree(ctx, target, options.Runner); err != nil {
				return 1, fmt.Errorf("%s: %w", selection.Workspaces[index].Name, err)
			}
		}
	}
	for index, target := range targets {
		logRemoveProgress(options.Stdout, selection.Workspaces[index], "cleaning generated workspace env")
		if err := removeGeneratedContextEnv(selection.Workspaces[index], target.WorktreePath); err != nil {
			return 1, fmt.Errorf("%s: %w", selection.Workspaces[index].Name, err)
		}
	}
	logRemoveProgress(options.Stdout, workspaceSpec{}, "closing tmux targets")
	if err := killWorkspaceTmux(ctx, selection, parsed.Branch, options); err != nil {
		return 1, err
	}
	for index, target := range targets {
		logRemoveProgress(options.Stdout, selection.Workspaces[index], "removing git worktree")
		if err := git.RemoveWorktreePath(ctx, target, parsed.Force, options.Runner); err != nil {
			return 1, fmt.Errorf("%s: %w", selection.Workspaces[index].Name, err)
		}
		logRemoveProgress(options.Stdout, selection.Workspaces[index], "deleting local branch")
		if err := git.DeleteBranch(ctx, target, parsed.Force, options.Runner); err != nil {
			return 1, fmt.Errorf("%s: %w", selection.Workspaces[index].Name, err)
		}
	}
	fmt.Fprintf(options.Stdout, "removed %s\n", parsed.Branch)
	return 0, nil
}

func runClose(ctx context.Context, args []string, options Options) (int, error) {
	parsed, err := parseCloseArgs(args)
	if err != nil {
		return 1, err
	}
	selection, err := resolveWorkspaceSelection(ctx, options, "", parsed.Workspaces)
	if err != nil {
		return 1, err
	}

	targets := make([]git.RemoveTarget, 0, len(selection.Workspaces))
	for _, workspace := range selection.Workspaces {
		target, err := git.ResolveCloseTarget(ctx, git.RemoveOptions{Branch: parsed.Branch, Cwd: workspace.RepoRoot, Runner: options.Runner})
		if err != nil {
			return 1, fmt.Errorf("%s: %w", workspace.Name, err)
		}
		targets = append(targets, target)
	}
	if !selection.AllWorkspaces {
		if err := ensureSingleWorkspaceLayout(parsed.Branch, "close", selection.Workspaces[0], targets[0]); err != nil {
			return 1, err
		}
	}
	if parsed.DryRun {
		fmt.Fprint(options.Stdout, renderCloseDryRun(selection, parsed.Branch, targets))
		return 0, nil
	}
	logCommandProgress(options.Stdout, "close", workspaceSpec{}, "closing tmux targets")
	if err := killWorkspaceTmux(ctx, selection, parsed.Branch, options); err != nil {
		return 1, err
	}
	fmt.Fprintf(options.Stdout, "closed %s\n", parsed.Branch)
	return 0, nil
}

type removeArgs struct {
	Branch     string
	Force      bool
	DryRun     bool
	Workspaces bool
}

func parseRemoveArgs(args []string) (removeArgs, error) {
	parsed := removeArgs{}
	positionals := []string{}
	for _, arg := range args {
		switch {
		case arg == "--force" || arg == "-f":
			parsed.Force = true
		case arg == "--dry-run":
			parsed.DryRun = true
		case arg == "--workspaces":
			parsed.Workspaces = true
		case strings.HasPrefix(arg, "-"):
			return removeArgs{}, fmt.Errorf("unknown option: %s", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) != 1 {
		return removeArgs{}, fmt.Errorf("usage: wktree remove [--force] [--dry-run] [--workspaces] <branch>")
	}
	parsed.Branch = positionals[0]
	return parsed, nil
}

type closeArgs struct {
	Branch     string
	DryRun     bool
	Workspaces bool
}

func parseCloseArgs(args []string) (closeArgs, error) {
	parsed := closeArgs{}
	positionals := []string{}
	for _, arg := range args {
		switch {
		case arg == "--dry-run":
			parsed.DryRun = true
		case arg == "--workspaces":
			parsed.Workspaces = true
		case strings.HasPrefix(arg, "-"):
			return closeArgs{}, fmt.Errorf("unknown option: %s", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) != 1 {
		return closeArgs{}, fmt.Errorf("usage: wktree close [--dry-run] [--workspaces] <branch>")
	}
	parsed.Branch = positionals[0]
	return parsed, nil
}

func renderRemoveDryRun(selection workspaceSelection, branch string, targets []git.RemoveTarget, force bool) string {
	worktreeRemove := "git worktree remove"
	branchDelete := "git branch -d"
	if force {
		worktreeRemove = "git worktree remove --force"
		branchDelete = "git branch -D"
	}

	var output strings.Builder
	fmt.Fprintln(&output, "dry run: remove")
	fmt.Fprintf(&output, "branch: %s\n", branch)
	fmt.Fprintf(&output, "tmux mode: %s\n", effectiveTmuxMode(selection))
	for index, target := range targets {
		workspace := selection.Workspaces[index]
		fmt.Fprintf(&output, "workspace: %s\n", workspace.Name)
		fmt.Fprintf(&output, "  worktree: %s\n", target.WorktreePath)
	}
	fmt.Fprintln(&output, "actions:")
	for _, action := range tmuxRemovalActions(selection, branch) {
		fmt.Fprintf(&output, "  %s\n", action)
	}
	for _, target := range targets {
		fmt.Fprintf(&output, "  %s %s\n", worktreeRemove, target.WorktreePath)
		fmt.Fprintf(&output, "  %s %s\n", branchDelete, target.Branch)
	}
	return output.String()
}

func renderCloseDryRun(selection workspaceSelection, branch string, targets []git.RemoveTarget) string {
	var output strings.Builder
	fmt.Fprintln(&output, "dry run: close")
	fmt.Fprintf(&output, "branch: %s\n", branch)
	fmt.Fprintf(&output, "tmux mode: %s\n", effectiveTmuxMode(selection))
	for index, target := range targets {
		workspace := selection.Workspaces[index]
		fmt.Fprintf(&output, "workspace: %s\n", workspace.Name)
		fmt.Fprintf(&output, "  worktree: %s\n", target.WorktreePath)
	}
	fmt.Fprintln(&output, "actions:")
	for _, action := range tmuxRemovalActions(selection, branch) {
		fmt.Fprintf(&output, "  %s\n", action)
	}
	return output.String()
}

func logRemoveProgress(writer io.Writer, workspace workspaceSpec, message string) {
	logCommandProgress(writer, "remove", workspace, message)
}

func logCommandProgress(writer io.Writer, command string, workspace workspaceSpec, message string) {
	if workspace.Name == "" {
		fmt.Fprintf(writer, "%s: %s\n", command, message)
		return
	}
	fmt.Fprintf(writer, "%s: %s: %s\n", command, workspace.Name, message)
}

func finishWorkspaceCommand(ctx context.Context, selection workspaceSelection, branch string, worktrees []workspaceWorktree, options Options) (int, error) {
	return finishWorkspaceCommandWithContext(ctx, selection, branch, worktrees, nil, options)
}

func finishWorkspaceCommandWithContext(ctx context.Context, selection workspaceSelection, branch string, worktrees []workspaceWorktree, prContext *setup.PullRequestContext, options Options) (int, error) {
	contexts := workspaceContexts(branch, worktrees)
	baseLogger := setup.Logger{Stdout: options.Stdout, Stderr: options.Stderr}
	status := 0
	plans := make([]workspacePlan, 0, len(worktrees))
	for _, worktree := range worktrees {
		files := config.WorkspaceFiles(selection.Config, worktree.Spec.Config)
		hooks := config.WorkspaceHooks(selection.Config, worktree.Spec.Config)
		context := contexts[worktree.Spec.Name]
		context.PullRequest = prContext
		plan := setup.NewPlan(worktree.Spec.WorkspaceRoot, workspacePath(worktree.Spec, worktree.Worktree.WorktreePath), worktree.Spec.Name, branch, files, hooks, worktree.Spec.Config.RandomizePorts, worktree.Spec.Config.SetEnv, worktree.Worktree.Reused, context)
		plans = append(plans, workspacePlan{plan: plan, logger: workspaceLogger(baseLogger, selection, worktree.Spec), open: append([]string(nil), worktree.Spec.Config.Open...)})
	}
	for _, item := range plans {
		if setup.RunPrepare(item.plan, item.logger) != 0 {
			status = 1
		}
	}
	for _, item := range plans {
		if setup.SetEnvFiles(item.plan, item.logger) != 0 {
			status = 1
		}
	}
	for _, item := range plans {
		if setup.WriteContextEnvLogged(item.plan, item.logger) != 0 {
			status = 1
		}
	}
	for _, item := range plans {
		if setup.RunPostCreate(ctx, item.plan, item.logger, options.ShellRunner) != 0 {
			status = 1
		}
	}
	windows := tmuxWindows(selection, branch, worktrees)
	openStatus, err := tmux.OpenLayout(ctx, tmux.LayoutOptions{
		Mode:        effectiveTmuxMode(selection),
		SessionName: sessionName(selection, branch),
		Windows:     windows,
		Env:         options.Env,
		Runner:      options.Runner,
	})
	if err != nil {
		return 1, err
	}
	if status != 0 {
		return status, nil
	}
	if openStatus != 0 {
		return openStatus, nil
	}
	openWorkspaceURLs(ctx, plans, options.Runner)
	return 0, nil
}

func workspaceLogger(base setup.Logger, selection workspaceSelection, workspace workspaceSpec) setup.Logger {
	if len(selection.Workspaces) > 1 || selection.AllWorkspaces {
		base.Prefix = workspace.Name
	}
	return base
}

func openWorkspaceURLs(ctx context.Context, plans []workspacePlan, runner run.Runner) {
	for _, item := range plans {
		for _, template := range item.open {
			url, err := setup.RenderSetEnvTemplate(template, item.plan.Context)
			if err != nil {
				item.logger.Warn("failed to resolve open URL %q: %s", template, err)
				continue
			}
			if err := openURL(ctx, url, runner); err != nil {
				item.logger.Warn("failed to open %s: %s", url, err)
				continue
			}
			item.logger.Info("opened %s", url)
		}
	}
}

func openURL(ctx context.Context, url string, runner run.Runner) error {
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	command, args := openCommand(url)
	result := runner.Run(ctx, command, args, run.Options{})
	if result.Err != nil || result.ExitCode != 0 {
		return fmt.Errorf("%s", run.FailureMessage(command, args, result))
	}
	return nil
}

func openCommand(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "cmd", []string{"/c", "start", "", url}
	default:
		return "xdg-open", []string{url}
	}
}

func ensurePullRequestTargetManaged(workspace workspaceSpec, target git.PullRequestTarget) error {
	if !target.BranchExists {
		return nil
	}
	context, ok, err := setup.ReadPullRequestContext(workspacePath(workspace, target.Worktree.WorktreePath))
	if err != nil {
		return err
	}
	if !ok || context.Number != target.PullRequest.Number || context.URL != target.PullRequest.URL {
		return fmt.Errorf("local branch already exists but is not managed for PR #%d: %s", target.PullRequest.Number, target.Worktree.Branch)
	}
	return nil
}

func resolveWorkspaceSelection(ctx context.Context, options Options, homeOption string, allWorkspaces bool) (workspaceSelection, error) {
	configRepoRoot, err := git.RepoRoot(ctx, options.Cwd, options.Runner)
	if err != nil {
		return workspaceSelection{}, err
	}
	configRepoSlug, err := git.RepoSlug(ctx, configRepoRoot, options.Runner)
	if err != nil {
		return workspaceSelection{}, err
	}
	homeDir, err := homeDir(options)
	if err != nil {
		return workspaceSelection{}, err
	}
	configPath, _, err := config.FindProjectPath(options.Cwd, configRepoRoot)
	if err != nil {
		return workspaceSelection{}, err
	}
	projectConfig, err := config.LoadProjectFile(configPath, homeDir)
	if err != nil {
		return workspaceSelection{}, err
	}
	configDir := filepath.Dir(configPath)
	if len(projectConfig.Workspaces) == 0 {
		projectConfig.Workspaces = []config.Workspace{{Name: defaultWorkspaceName(configRepoRoot, configRepoSlug, configDir)}}
	}

	workspaces := projectConfig.Workspaces
	selectedAllWorkspaces := allWorkspaces || projectConfig.WorkspaceMode == config.WorkspaceModeAll
	if !selectedAllWorkspaces {
		workspaces = workspaces[:1]
	}

	selected := make([]workspaceSpec, 0, len(workspaces))
	seenRepos := map[string]string{}
	for _, workspace := range workspaces {
		repoPath := configDir
		if workspace.Repo != "" {
			repoPath, err = config.ExpandConfiguredPath(workspace.Repo, homeDir, configDir)
			if err != nil {
				return workspaceSelection{}, fmt.Errorf("%s repo: %w", workspace.Name, err)
			}
		}
		repoRoot, err := git.RepoRoot(ctx, repoPath, options.Runner)
		if err != nil {
			return workspaceSelection{}, fmt.Errorf("%s repo: %w", workspace.Name, err)
		}
		if previous, ok := seenRepos[cleanAbsPath(repoRoot)]; ok {
			return workspaceSelection{}, fmt.Errorf("workspaces %q and %q resolve to the same repo: %s", previous, workspace.Name, repoRoot)
		}
		seenRepos[cleanAbsPath(repoRoot)] = workspace.Name
		spec, err := newWorkspaceSpec(workspace.Name, repoRoot, repoPath, workspace)
		if err != nil {
			return workspaceSelection{}, fmt.Errorf("%s repo: %w", workspace.Name, err)
		}
		selected = append(selected, spec)
	}

	worktreeHome := homeOption
	if worktreeHome == "" {
		worktreeHome = projectConfig.WorktreeDir
	}
	if worktreeHome != "" {
		worktreeHome, err = config.ExpandConfiguredPath(worktreeHome, homeDir, configDir)
		if err != nil {
			return workspaceSelection{}, fmt.Errorf("worktree_dir: %w", err)
		}
	}

	return workspaceSelection{
		Config:         projectConfig,
		ConfigPath:     configPath,
		ConfigDir:      configDir,
		ConfigRepoRoot: configRepoRoot,
		ConfigRepoSlug: configRepoSlug,
		WorktreeHome:   worktreeHome,
		Workspaces:     selected,
		AllWorkspaces:  selectedAllWorkspaces,
	}, nil
}

func resolvePullRequestWorkspaceSelection(ctx context.Context, options Options, homeOption string) (workspaceSelection, error) {
	configRepoRoot, err := git.RepoRoot(ctx, options.Cwd, options.Runner)
	if err != nil {
		return workspaceSelection{}, err
	}
	configRepoSlug, err := git.RepoSlug(ctx, configRepoRoot, options.Runner)
	if err != nil {
		return workspaceSelection{}, err
	}
	homeDir, err := homeDir(options)
	if err != nil {
		return workspaceSelection{}, err
	}
	configPath, _, err := config.FindProjectPath(options.Cwd, configRepoRoot)
	if err != nil {
		return workspaceSelection{}, err
	}
	projectConfig, err := config.LoadProjectFile(configPath, homeDir)
	if err != nil {
		return workspaceSelection{}, err
	}
	configDir := filepath.Dir(configPath)

	matches := []workspaceSpec{}
	if len(projectConfig.Workspaces) == 0 {
		repoPath := configDir
		repoRoot, err := git.RepoRoot(ctx, repoPath, options.Runner)
		if err != nil {
			return workspaceSelection{}, err
		}
		name := defaultWorkspaceName(configRepoRoot, configRepoSlug, configDir)
		spec, err := newWorkspaceSpec(name, repoRoot, repoPath, config.Workspace{Name: name})
		if err != nil {
			return workspaceSelection{}, err
		}
		matches = append(matches, spec)
	} else {
		seenRepos := map[string]string{}
		for _, workspace := range projectConfig.Workspaces {
			repoPath := configDir
			if workspace.Repo != "" {
				repoPath, err = config.ExpandConfiguredPath(workspace.Repo, homeDir, configDir)
				if err != nil {
					return workspaceSelection{}, fmt.Errorf("%s repo: %w", workspace.Name, err)
				}
			}
			repoRoot, err := git.RepoRoot(ctx, repoPath, options.Runner)
			if err != nil {
				return workspaceSelection{}, fmt.Errorf("%s repo: %w", workspace.Name, err)
			}
			if previous, ok := seenRepos[cleanAbsPath(repoRoot)]; ok {
				return workspaceSelection{}, fmt.Errorf("workspaces %q and %q resolve to the same repo: %s", previous, workspace.Name, repoRoot)
			}
			seenRepos[cleanAbsPath(repoRoot)] = workspace.Name
			if samePath(repoRoot, configRepoRoot) {
				spec, err := newWorkspaceSpec(workspace.Name, repoRoot, repoPath, workspace)
				if err != nil {
					return workspaceSelection{}, fmt.Errorf("%s repo: %w", workspace.Name, err)
				}
				matches = append(matches, spec)
			}
		}
	}
	if len(matches) == 0 {
		repoPath := configDir
		repoRoot, err := git.RepoRoot(ctx, repoPath, options.Runner)
		if err != nil {
			return workspaceSelection{}, err
		}
		name := defaultWorkspaceName(configRepoRoot, configRepoSlug, configDir)
		spec, err := newWorkspaceSpec(name, repoRoot, repoPath, config.Workspace{Name: name})
		if err != nil {
			return workspaceSelection{}, err
		}
		matches = append(matches, spec)
	}

	worktreeHome := homeOption
	if worktreeHome == "" {
		worktreeHome = projectConfig.WorktreeDir
	}
	if worktreeHome != "" {
		worktreeHome, err = config.ExpandConfiguredPath(worktreeHome, homeDir, configDir)
		if err != nil {
			return workspaceSelection{}, fmt.Errorf("worktree_dir: %w", err)
		}
	}

	return workspaceSelection{
		Config:         projectConfig,
		ConfigPath:     configPath,
		ConfigDir:      configDir,
		ConfigRepoRoot: configRepoRoot,
		ConfigRepoSlug: configRepoSlug,
		WorktreeHome:   worktreeHome,
		Workspaces:     matches[:1],
		AllWorkspaces:  false,
	}, nil
}

func workspaceContexts(branch string, worktrees []workspaceWorktree) map[string]setup.Context {
	workspacePaths := map[string]string{}
	for _, worktree := range worktrees {
		workspacePaths[worktree.Spec.Name] = workspacePath(worktree.Spec, worktree.Worktree.WorktreePath)
	}
	contexts := map[string]setup.Context{}
	for _, worktree := range worktrees {
		contexts[worktree.Spec.Name] = setup.Context{
			WorkspacePaths: workspacePaths,
		}
	}
	return contexts
}

func tmuxWindows(selection workspaceSelection, branch string, worktrees []workspaceWorktree) []tmux.Window {
	windows := make([]tmux.Window, 0, len(worktrees))
	for _, worktree := range worktrees {
		windows = append(windows, tmux.Window{
			Name:         workspaceWindowName(worktree.Spec.Name),
			WorktreePath: workspacePath(worktree.Spec, worktree.Worktree.WorktreePath),
			Commands:     paneCommands(config.WorkspacePanes(worktree.Spec.Config)),
		})
	}
	return windows
}

func paneCommands(commands []config.PaneCommand) []tmux.PaneCommand {
	converted := make([]tmux.PaneCommand, 0, len(commands))
	for _, command := range commands {
		converted = append(converted, tmux.PaneCommand{
			Command:    command.Command,
			Commands:   append([]string(nil), command.Commands...),
			Split:      command.Split,
			Focus:      command.Focus,
			Zoom:       command.Zoom,
			Size:       command.Size,
			Percentage: command.Percentage,
		})
	}
	return converted
}

func killWorkspaceTmux(ctx context.Context, selection workspaceSelection, branch string, options Options) error {
	mode := effectiveTmuxMode(selection)
	return tmux.KillLayout(ctx, tmux.KillOptions{
		Mode:        mode,
		SessionName: sessionName(selection, branch),
		WindowNames: workspaceWindowNames(selection),
		KillSession: mode == tmux.ModeSession && selection.AllWorkspaces,
		Env:         options.Env,
		Runner:      options.Runner,
	})
}

func ensureSingleWorkspaceLayout(branch string, command string, workspace workspaceSpec, target git.RemoveTarget) error {
	count, err := setup.ContextEnvWorkspaceDirCount(workspacePath(workspace, target.WorktreePath))
	if err != nil {
		return err
	}
	if count > 1 {
		return fmt.Errorf("branch appears to use multiple workspaces from .wktree.env; rerun with --workspaces to %s all: %s", command, branch)
	}
	return nil
}

func newWorkspaceSpec(name string, repoRoot string, workspaceRoot string, workspace config.Workspace) (workspaceSpec, error) {
	absoluteRoot := cleanAbsPath(repoRoot)
	absoluteWorkspace := cleanAbsPath(workspaceRoot)
	relative, err := filepath.Rel(absoluteRoot, absoluteWorkspace)
	if err != nil {
		return workspaceSpec{}, err
	}
	if relative == "." {
		relative = ""
	}
	if relative != "" && (relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative)) {
		return workspaceSpec{}, fmt.Errorf("path must be inside git repo root: %s", workspaceRoot)
	}
	return workspaceSpec{
		Name:             name,
		RepoRoot:         repoRoot,
		WorkspaceRoot:    absoluteWorkspace,
		WorkspaceRelPath: relative,
		Config:           workspace,
	}, nil
}

func workspacePath(workspace workspaceSpec, worktreePath string) string {
	if workspace.WorkspaceRelPath == "" {
		return worktreePath
	}
	return filepath.Join(worktreePath, workspace.WorkspaceRelPath)
}

func defaultWorkspaceName(repoRoot string, repoSlug string, configDir string) string {
	if !samePath(configDir, repoRoot) {
		name := strings.TrimSpace(filepath.Base(filepath.Clean(configDir)))
		if name != "" && name != "." && name != string(filepath.Separator) {
			return name
		}
	}
	return repoSlug
}

func removeGeneratedContextEnv(workspace workspaceSpec, worktreePath string) error {
	workspaceDir := workspacePath(workspace, worktreePath)
	if err := setup.RemoveContextEnv(workspaceDir); err != nil {
		return err
	}
	if samePath(workspaceDir, worktreePath) {
		return nil
	}
	return setup.RemoveContextEnv(worktreePath)
}

func tmuxRemovalActions(selection workspaceSelection, branch string) []string {
	mode := effectiveTmuxMode(selection)
	if mode == tmux.ModeSession && selection.AllWorkspaces {
		return []string{"tmux kill-session -t " + sessionName(selection, branch) + " if it exists"}
	}
	actions := []string{}
	for _, windowName := range workspaceWindowNames(selection) {
		target := windowName
		if mode == tmux.ModeSession {
			target = sessionName(selection, branch) + ":" + windowName
		}
		actions = append(actions, "tmux kill-window -t "+target+" if it exists")
	}
	return actions
}

func effectiveTmuxMode(selection workspaceSelection) string {
	if selection.AllWorkspaces {
		return tmux.ModeSession
	}
	return selection.Config.TmuxMode
}

func workspaceWindowNames(selection workspaceSelection) []string {
	names := make([]string, 0, len(selection.Workspaces))
	for _, workspace := range selection.Workspaces {
		names = append(names, workspaceWindowName(workspace.Name))
	}
	return names
}

func workspaceWindowName(workspaceName string) string {
	return tmux.TargetName(workspaceName)
}

func sessionName(selection workspaceSelection, branch string) string {
	branchSlug, err := paths.BranchSlug(branch)
	if err != nil {
		branchSlug = "branch"
	}
	return tmux.TargetName(selection.ConfigRepoSlug) + "/" + tmux.TargetName(branchSlug)
}

type worktreeArgs struct {
	Branch      string
	From        string
	Home        string
	PullRequest string
	Workspaces  bool
}

func parseWorktreeArgs(command string, args []string) (worktreeArgs, error) {
	parsed := worktreeArgs{}
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--workspaces":
			parsed.Workspaces = true
		case arg == "--pr":
			if command != "switch" {
				return worktreeArgs{}, fmt.Errorf("unknown option: %s", arg)
			}
			i++
			if i >= len(args) || args[i] == "" || strings.HasPrefix(args[i], "-") {
				return worktreeArgs{}, fmt.Errorf("%s", usageFor(command))
			}
			parsed.PullRequest = args[i]
		case strings.HasPrefix(arg, "--pr="):
			if command != "switch" {
				return worktreeArgs{}, fmt.Errorf("unknown option: --pr")
			}
			parsed.PullRequest = strings.TrimPrefix(arg, "--pr=")
			if parsed.PullRequest == "" || strings.HasPrefix(parsed.PullRequest, "-") {
				return worktreeArgs{}, fmt.Errorf("%s", usageFor(command))
			}
		case arg == "--home":
			i++
			if i >= len(args) {
				return worktreeArgs{}, fmt.Errorf("%s", usageFor(command))
			}
			parsed.Home = args[i]
		case strings.HasPrefix(arg, "--home="):
			parsed.Home = strings.TrimPrefix(arg, "--home=")
		case arg == "--from":
			if command != "new" {
				return worktreeArgs{}, fmt.Errorf("unknown option: %s", arg)
			}
			i++
			if i >= len(args) || args[i] == "" || strings.HasPrefix(args[i], "-") {
				return worktreeArgs{}, fmt.Errorf("%s", usageFor(command))
			}
			parsed.From = args[i]
		case strings.HasPrefix(arg, "--from="):
			if command != "new" {
				return worktreeArgs{}, fmt.Errorf("unknown option: --from")
			}
			parsed.From = strings.TrimPrefix(arg, "--from=")
			if parsed.From == "" || strings.HasPrefix(parsed.From, "-") {
				return worktreeArgs{}, fmt.Errorf("%s", usageFor(command))
			}
		case arg == "--tmux", arg == "--no-cd", arg == "--no-setup":
			return worktreeArgs{}, fmt.Errorf("unknown option: %s", arg)
		case strings.HasPrefix(arg, "-"):
			return worktreeArgs{}, fmt.Errorf("unknown option: %s", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if parsed.PullRequest != "" {
		if parsed.Workspaces {
			return worktreeArgs{}, fmt.Errorf("--pr cannot be used with --workspaces")
		}
		if len(positionals) != 0 {
			return worktreeArgs{}, fmt.Errorf("%s", usageFor(command))
		}
		return parsed, nil
	}
	if len(positionals) != 1 {
		return worktreeArgs{}, fmt.Errorf("%s", usageFor(command))
	}
	parsed.Branch = positionals[0]
	return parsed, nil
}

func usageFor(command string) string {
	if command == "new" {
		return "usage: wktree new [--home <path>] [--from <ref>] [--workspaces] <branch>"
	}
	if command == "switch" {
		return "usage: wktree switch [--home <path>] [--workspaces] <branch> | wktree switch [--home <path>] --pr <number|url>"
	}
	return fmt.Sprintf("usage: wktree %s [--home <path>] [--workspaces] <branch>", command)
}

func runInit(ctx context.Context, args []string, options Options) (int, error) {
	if len(args) == 1 && (args[0] == "zsh" || args[0] == "bash") {
		return runCompletion(args, options)
	}
	if len(args) != 0 {
		return 1, fmt.Errorf("usage: wktree init")
	}
	if _, err := git.RepoRoot(ctx, options.Cwd, options.Runner); err != nil {
		return 1, err
	}
	configPath, err := config.WriteProjectTemplate(cleanAbsPath(options.Cwd))
	if err != nil {
		return 1, err
	}
	fmt.Fprintf(options.Stdout, "created %s\n", configPath)
	return 0, nil
}

func runCompletion(args []string, options Options) (int, error) {
	if len(args) != 1 {
		return 1, fmt.Errorf("usage: wktree completion <zsh|bash>")
	}
	initScript, err := shellinit.Generate(args[0])
	if err != nil {
		return 1, err
	}
	fmt.Fprint(options.Stdout, initScript)
	return 0, nil
}

type listRow struct {
	current string
	branch  string
	head    string
	path    string
}

func renderWorktreeList(worktreeList git.WorktreeList) string {
	rows := make([]listRow, 0, len(worktreeList.Worktrees))
	widths := []int{len("CURRENT"), len("BRANCH"), len("HEAD")}
	for _, worktree := range worktreeList.Worktrees {
		row := listRow{
			branch: displayBranch(worktree),
			head:   shortHead(worktree.Head),
			path:   worktree.Path,
		}
		if samePath(worktreeList.CurrentPath, worktree.Path) {
			row.current = "*"
		}
		widths[0] = max(widths[0], len(row.current))
		widths[1] = max(widths[1], len(row.branch))
		widths[2] = max(widths[2], len(row.head))
		rows = append(rows, row)
	}

	var output strings.Builder
	fmt.Fprintf(&output, "%-*s  %-*s  %-*s  %s\n", widths[0], "CURRENT", widths[1], "BRANCH", widths[2], "HEAD", "PATH")
	for _, row := range rows {
		fmt.Fprintf(&output, "%-*s  %-*s  %-*s  %s\n", widths[0], row.current, widths[1], row.branch, widths[2], row.head, row.path)
	}
	return output.String()
}

func displayBranch(worktree git.ListedWorktree) string {
	if worktree.Detached || worktree.Branch == "" {
		return "(detached)"
	}
	return worktree.Branch
}

func shortHead(head string) string {
	if len(head) <= 7 {
		return head
	}
	return head[:7]
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

func max(left int, right int) int {
	if left > right {
		return left
	}
	return right
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

func HelpText() string {
	return `wktree - create strict Git worktrees

Usage:
  wktree --help
  wktree --version
  wktree init
  wktree doctor
  wktree list
  wktree new [--home <path>] [--from <ref>] [--workspaces] <branch>
  wktree close [--dry-run] [--workspaces] <branch>
  wktree remove [--force] [--dry-run] [--workspaces] <branch>
  wktree switch [--home <path>] [--workspaces] <branch>
  wktree switch [--home <path>] --pr <number|url>
  wktree completion zsh
  wktree completion bash

Examples:
  wktree init
  wktree doctor
  wktree list
  wktree new feature/example
  wktree new --from main feature/example
  wktree new --workspaces feature/example
  wktree switch --pr 123
  wktree close --workspaces feature/example
  wktree remove --dry-run --workspaces feature/example
  wktree switch --workspaces feature/example
  eval "$(wktree completion zsh)"

Setup config:
  Create:  wktree init
  Project: nearest .wktree.yaml
  Keys:    worktree_dir, tmux_mode, workspace_mode, defaults, workspaces, panes, files, hooks, set_env, open

Shell integration:
  eval "$(wktree completion zsh)"
  eval "$(wktree completion bash)"
`
}

func normalizeOptions(options Options) Options {
	if options.Cwd == "" {
		if cwd, err := os.Getwd(); err == nil {
			options.Cwd = cwd
		}
	}
	if options.Env == nil {
		options.Env = envMap(os.Environ())
	}
	if options.Stdout == nil {
		options.Stdout = os.Stdout
	}
	if options.Stderr == nil {
		options.Stderr = os.Stderr
	}
	if options.Runner == nil {
		options.Runner = run.DefaultRunner{}
	}
	if options.ShellRunner == nil {
		options.ShellRunner = setup.DefaultShellRunner{}
	}
	return options
}

func homeDir(options Options) (string, error) {
	if options.Env != nil && options.Env["HOME"] != "" {
		return options.Env["HOME"], nil
	}
	return os.UserHomeDir()
}

func envMap(env []string) map[string]string {
	values := map[string]string{}
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	return values
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
