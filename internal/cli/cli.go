package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	Name     string
	RepoRoot string
	Config   config.Workspace
}

type workspaceSelection struct {
	Config         config.Config
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
	case "remove":
		if strings.HasPrefix(prefix, "-") {
			values = filterPrefix([]string{"--force", "--dry-run", "--workspaces"}, prefix)
		} else {
			values, err = git.CompleteRemoveBranches(ctx, options.Cwd, prefix, options.Runner)
		}
	case "switch":
		values, err = completeWorktreeCommand(ctx, options, prefix, []string{"--home", "--workspaces"})
	default:
		values = filterPrefix([]string{"doctor", "list", "new", "remove", "switch", "init", "completion"}, prefix)
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
		fmt.Fprintf(options.Stdout, "[ok] project config path: %s\n", config.ProjectPath(selection.ConfigRepoRoot))
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
		if err := ensureSingleWorkspaceRemove(parsed.Branch, targets[0]); err != nil {
			return 1, err
		}
	}
	if parsed.DryRun {
		fmt.Fprint(options.Stdout, renderRemoveDryRun(selection, parsed.Branch, targets, parsed.Force))
		return 0, nil
	}
	if !parsed.Force {
		for index, target := range targets {
			if err := git.EnsureCleanWorktree(ctx, target, options.Runner); err != nil {
				return 1, fmt.Errorf("%s: %w", selection.Workspaces[index].Name, err)
			}
		}
	}
	if err := killWorkspaceTmux(ctx, selection, parsed.Branch, options); err != nil {
		return 1, err
	}
	for index, target := range targets {
		if err := setup.RemoveContextEnv(target.WorktreePath); err != nil {
			return 1, fmt.Errorf("%s: %w", selection.Workspaces[index].Name, err)
		}
		if err := git.RemoveWorktree(ctx, target, parsed.Force, options.Runner); err != nil {
			return 1, fmt.Errorf("%s: %w", selection.Workspaces[index].Name, err)
		}
	}
	fmt.Fprintf(options.Stdout, "removed %s\n", parsed.Branch)
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

func finishWorkspaceCommand(ctx context.Context, selection workspaceSelection, branch string, worktrees []workspaceWorktree, options Options) (int, error) {
	contexts := workspaceContexts(branch, worktrees)
	logger := setup.Logger{Stdout: options.Stdout, Stderr: options.Stderr}
	status := 0
	for _, worktree := range worktrees {
		files := config.WorkspaceFiles(selection.Config, worktree.Spec.Config)
		hooks := config.WorkspaceHooks(selection.Config, worktree.Spec.Config)
		plan := setup.NewPlan(worktree.Worktree.RepoRoot, worktree.Worktree.WorktreePath, worktree.Spec.Name, branch, files, hooks, worktree.Spec.Config.RandomizePorts, worktree.Worktree.Reused, contexts[worktree.Spec.Name])
		if setup.Run(ctx, plan, logger, options.ShellRunner) != 0 {
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
	return openStatus, nil
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
	projectConfig, err := config.LoadProject(configRepoRoot, homeDir)
	if err != nil {
		return workspaceSelection{}, err
	}
	if len(projectConfig.Workspaces) == 0 {
		projectConfig.Workspaces = []config.Workspace{{Name: configRepoSlug}}
	}

	configDir := filepath.Dir(config.ProjectPath(configRepoRoot))
	workspaces := projectConfig.Workspaces
	selectedAllWorkspaces := allWorkspaces || projectConfig.WorkspaceMode == config.WorkspaceModeAll
	if !selectedAllWorkspaces {
		workspaces = workspaces[:1]
	}

	selected := make([]workspaceSpec, 0, len(workspaces))
	seenRepos := map[string]string{}
	for _, workspace := range workspaces {
		repoPath := configRepoRoot
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
		selected = append(selected, workspaceSpec{Name: workspace.Name, RepoRoot: repoRoot, Config: workspace})
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
		ConfigRepoRoot: configRepoRoot,
		ConfigRepoSlug: configRepoSlug,
		WorktreeHome:   worktreeHome,
		Workspaces:     selected,
		AllWorkspaces:  selectedAllWorkspaces,
	}, nil
}

func workspaceContexts(branch string, worktrees []workspaceWorktree) map[string]setup.Context {
	workspacePaths := map[string]string{}
	for _, worktree := range worktrees {
		workspacePaths[worktree.Spec.Name] = worktree.Worktree.WorktreePath
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
			WorktreePath: worktree.Worktree.WorktreePath,
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

func ensureSingleWorkspaceRemove(branch string, target git.RemoveTarget) error {
	count, err := setup.ContextEnvWorkspaceDirCount(target.WorktreePath)
	if err != nil {
		return err
	}
	if count > 1 {
		return fmt.Errorf("branch appears to use multiple workspaces from .wktree.env; rerun with --workspaces to remove all: %s", branch)
	}
	return nil
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
	Branch     string
	From       string
	Home       string
	Workspaces bool
}

func parseWorktreeArgs(command string, args []string) (worktreeArgs, error) {
	parsed := worktreeArgs{}
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--workspaces":
			parsed.Workspaces = true
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
	return fmt.Sprintf("usage: wktree %s [--home <path>] [--workspaces] <branch>", command)
}

func runInit(ctx context.Context, args []string, options Options) (int, error) {
	if len(args) == 1 && (args[0] == "zsh" || args[0] == "bash") {
		return runCompletion(args, options)
	}
	if len(args) != 0 {
		return 1, fmt.Errorf("usage: wktree init")
	}
	repoRoot, err := git.RepoRoot(ctx, options.Cwd, options.Runner)
	if err != nil {
		return 1, err
	}
	configPath, err := config.WriteProjectTemplate(repoRoot)
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
  wktree remove [--force] [--dry-run] [--workspaces] <branch>
  wktree switch [--home <path>] [--workspaces] <branch>
  wktree completion zsh
  wktree completion bash

Examples:
  wktree init
  wktree doctor
  wktree list
  wktree new feature/example
  wktree new --from main feature/example
  wktree new --workspaces feature/example
  wktree remove --dry-run --workspaces feature/example
  wktree switch --workspaces feature/example
  eval "$(wktree completion zsh)"

Setup config:
  Create:  wktree init
  Project: .wktree.yaml
  Keys:    worktree_dir, tmux_mode, workspace_mode, defaults, workspaces, panes, files, hooks

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
