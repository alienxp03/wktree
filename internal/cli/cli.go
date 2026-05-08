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
	case "list":
		exitCode, err = runList(ctx, args[1:], app)
	case "new":
		exitCode, err = runNew(ctx, args[1:], app)
	case "remove":
		exitCode, err = runRemove(ctx, args[1:], app)
	case "switch":
		exitCode, err = runSwitch(ctx, args[1:], app)
	case "init":
		exitCode, err = runInit(args[1:], app)
	case "__setup":
		exitCode, err = runDeferredSetup(ctx, args[1:], app)
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
	case "init":
		values = filterPrefix([]string{"bash", "zsh"}, prefix)
	case "remove":
		values, err = git.CompleteRemoveBranches(ctx, options.Cwd, prefix, options.Runner)
	case "switch":
		values, err = git.CompleteSwitchBranches(ctx, options.Cwd, prefix, options.Runner)
	default:
		values = filterPrefix([]string{"list", "new", "remove", "switch", "init"}, prefix)
	}
	if err != nil {
		return 1, err
	}
	for _, value := range values {
		fmt.Fprintln(options.Stdout, value)
	}
	return 0, nil
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

func runRemove(ctx context.Context, args []string, options Options) (int, error) {
	parsed, err := parseRemoveArgs(args)
	if err != nil {
		return 1, err
	}
	target, err := git.ResolveRemoveTarget(ctx, git.RemoveOptions{Branch: parsed.Branch, Cwd: options.Cwd, Force: parsed.Force, Runner: options.Runner})
	if err != nil {
		return 1, err
	}
	if err := tmux.KillSessionForWorktree(ctx, target.WorktreePath, options.Runner); err != nil {
		return 1, err
	}
	if err := git.RemoveWorktree(ctx, target, parsed.Force, options.Runner); err != nil {
		return 1, err
	}
	fmt.Fprintf(options.Stdout, "removed %s\n", target.Branch)
	return 0, nil
}

type removeArgs struct {
	Branch string
	Force  bool
}

func parseRemoveArgs(args []string) (removeArgs, error) {
	parsed := removeArgs{}
	positionals := []string{}
	for _, arg := range args {
		switch {
		case arg == "--force" || arg == "-f":
			parsed.Force = true
		case strings.HasPrefix(arg, "-"):
			return removeArgs{}, fmt.Errorf("unknown option: %s", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if len(positionals) != 1 {
		return removeArgs{}, fmt.Errorf("usage: wktree remove [--force] <branch>")
	}
	parsed.Branch = positionals[0]
	return parsed, nil
}

func runNew(ctx context.Context, args []string, options Options) (int, error) {
	parsed, err := parseWorktreeArgs("new", args)
	if err != nil {
		return 1, err
	}

	setupConfig := config.Config{}
	if !parsed.NoSetup {
		repoRoot, err := git.RepoRoot(ctx, options.Cwd, options.Runner)
		if err != nil {
			return 1, err
		}
		setupConfig, err = loadSetupConfig(repoRoot, options)
		if err != nil {
			return 1, err
		}
	}

	worktree, err := git.CreateWorktree(ctx, git.CreateOptions{
		Branch:     parsed.Branch,
		HomeOption: parsed.Home,
		Cwd:        options.Cwd,
		Runner:     options.Runner,
	})
	if err != nil {
		return 1, err
	}
	return finishWorktreeCommand(ctx, parsed, worktree, setupConfig, options)
}

func runSwitch(ctx context.Context, args []string, options Options) (int, error) {
	parsed, err := parseWorktreeArgs("switch", args)
	if err != nil {
		return 1, err
	}

	target, err := git.ResolveSwitchWorktree(ctx, git.CreateOptions{
		Branch:     parsed.Branch,
		HomeOption: parsed.Home,
		Cwd:        options.Cwd,
		Runner:     options.Runner,
	})
	if err != nil {
		return 1, err
	}

	setupConfig := config.Config{}
	if !parsed.NoSetup && !target.Worktree.Reused {
		setupConfig, err = loadSetupConfig(target.Worktree.RepoRoot, options)
		if err != nil {
			return 1, err
		}
	}

	worktree, err := git.SwitchWorktree(ctx, target, options.Runner)
	if err != nil {
		return 1, err
	}
	return finishWorktreeCommand(ctx, parsed, worktree, setupConfig, options)
}

func loadSetupConfig(repoRoot string, options Options) (config.Config, error) {
	return config.LoadMerged(repoRoot, config.LoadOptions{Env: options.Env})
}

func finishWorktreeCommand(ctx context.Context, parsed worktreeArgs, worktree git.Worktree, setupConfig config.Config, options Options) (int, error) {
	setupPlan := setup.NewPlan(worktree.RepoRoot, worktree.WorktreePath, setupConfig)
	logger := setup.Logger{Stdout: options.Stdout, Stderr: options.Stderr}

	if parsed.Tmux {
		return tmux.Open(ctx, tmux.Options{
			WorktreePath: worktree.WorktreePath,
			RepoSlug:     worktree.RepoSlug,
			BranchSlug:   worktree.BranchSlug,
			SetupPlan:    setupPlan,
			Env:          options.Env,
			Runner:       options.Runner,
			Logger:       logger,
		})
	}

	if !parsed.NoCD {
		if cdFile := options.Env["WKTREE_CD_FILE"]; cdFile != "" {
			if err := os.WriteFile(cdFile, []byte(worktree.WorktreePath+"\n"), 0o600); err != nil {
				return 1, err
			}
		}
	}

	if parsed.NoSetup || !config.HasSetup(setupConfig) {
		return 0, nil
	}
	if !parsed.NoCD {
		if setupFile := options.Env["WKTREE_SETUP_FILE"]; setupFile != "" {
			return 0, setup.WritePlan(setupFile, setupPlan)
		}
	}
	return setup.Run(ctx, setupPlan, logger, options.ShellRunner), nil
}

func runInit(args []string, options Options) (int, error) {
	if len(args) != 1 {
		return 1, fmt.Errorf("usage: wktree init <zsh|bash>")
	}
	initScript, err := shellinit.Generate(args[0])
	if err != nil {
		return 1, err
	}
	fmt.Fprint(options.Stdout, initScript)
	return 0, nil
}

func runDeferredSetup(ctx context.Context, args []string, options Options) (int, error) {
	if len(args) != 1 {
		return 1, fmt.Errorf("usage: wktree __setup <plan-file>")
	}
	setupPlan, err := setup.ReadPlan(args[0])
	if err != nil {
		return 1, err
	}
	return setup.Run(ctx, setupPlan, setup.Logger{Stdout: options.Stdout, Stderr: options.Stderr}, options.ShellRunner), nil
}

type worktreeArgs struct {
	Branch  string
	Home    string
	Tmux    bool
	NoCD    bool
	NoSetup bool
}

func parseWorktreeArgs(command string, args []string) (worktreeArgs, error) {
	parsed := worktreeArgs{}
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--tmux":
			parsed.Tmux = true
		case arg == "--no-cd":
			parsed.NoCD = true
		case arg == "--no-setup":
			parsed.NoSetup = true
		case arg == "--home":
			i++
			if i >= len(args) {
				return worktreeArgs{}, fmt.Errorf("%s", usageFor(command))
			}
			parsed.Home = args[i]
		case strings.HasPrefix(arg, "--home="):
			parsed.Home = strings.TrimPrefix(arg, "--home=")
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
	return fmt.Sprintf("usage: wktree %s [--tmux] [--home <path>] [--no-cd] [--no-setup] <branch>", command)
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
  wktree list
  wktree new [--tmux] [--home <path>] [--no-cd] [--no-setup] <branch>
  wktree remove [--force] <branch>
  wktree switch [--tmux] [--home <path>] [--no-cd] [--no-setup] <branch>
  wktree init zsh
  wktree init bash

Examples:
  wktree list
  wktree new feature/example
  wktree remove feature/example
  wktree switch feature/example
  wktree new --home /tmp/worktrees feature/example
  wktree new --tmux feature/example
  eval "$(wktree init zsh)"

Environment:
  WKTREE_CD_FILE        Used internally by shell integration.

Setup config:
  Global:  $XDG_CONFIG_HOME/wktree/config.yaml or ~/.config/wktree/config.yaml
  Project: .wktree.yaml
  Keys:    copy, symlink, postSetup

Shell integration:
  eval "$(wktree init zsh)"
  eval "$(wktree init bash)"
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
