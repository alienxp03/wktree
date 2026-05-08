package tmux

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alienxp03/wktree/internal/run"
	"github.com/alienxp03/wktree/internal/setup"
)

var unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

type Options struct {
	WorktreePath string
	RepoSlug     string
	BranchSlug   string
	SetupPlan    setup.Plan
	Env          map[string]string
	Runner       run.Runner
	Logger       setup.Logger
}

func Open(ctx context.Context, options Options) (int, error) {
	runner := options.Runner
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	if err := ensureAvailable(ctx, runner); err != nil {
		return 1, err
	}
	return openSession(ctx, runner, options)
}

func KillSessionForWorktree(ctx context.Context, worktreePath string, runner run.Runner) error {
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	if err := ensureAvailable(ctx, runner); err != nil {
		return nil
	}
	sessionName := SessionName(worktreePath)
	hasSessionArgs := []string{"has-session", "-t", sessionName}
	hasSession := runTmux(ctx, runner, hasSessionArgs, true, false)
	if hasSession.ExitCode == 1 {
		return nil
	}
	if hasSession.ExitCode != 0 {
		return errors.New(run.FailureMessage("tmux", hasSessionArgs, hasSession))
	}
	killArgs := []string{"kill-session", "-t", sessionName}
	killed := runTmux(ctx, runner, killArgs, false, false)
	if killed.Err != nil || killed.ExitCode != 0 {
		return errors.New(run.FailureMessage("tmux", killArgs, killed))
	}
	return nil
}

func SessionName(worktreePath string) string {
	current := pathNameComponent(filepath.Base(filepath.Clean(worktreePath)))
	parent := pathNameComponent(filepath.Base(filepath.Dir(filepath.Clean(worktreePath))))
	if current == "" {
		return "wktree"
	}
	if parent == "" {
		return current
	}
	return parent + "/" + current
}

func WindowName(worktreePath string) string {
	name := pathNameComponent(filepath.Base(filepath.Clean(worktreePath)))
	if name == "" {
		return "wktree"
	}
	return name
}

func VisibleCommand(command string) string {
	return "printf '%s\\n' " + singleQuote("$ "+command) + "; " + command + " || printf '%s\\n' " + singleQuote("warning: post setup command failed: "+command) + " >&2"
}

func openSession(ctx context.Context, runner run.Runner, options Options) (int, error) {
	sessionName := SessionName(options.WorktreePath)
	windowName := WindowName(options.WorktreePath)
	hasSessionArgs := []string{"has-session", "-t", sessionName}
	hasSession := runTmux(ctx, runner, hasSessionArgs, true, false)
	if hasSession.ExitCode == 0 {
		return 1, fmt.Errorf("tmux session already exists: %s", sessionName)
	}
	if hasSession.ExitCode != 1 {
		return 1, errors.New(run.FailureMessage("tmux", hasSessionArgs, hasSession))
	}

	newSessionArgs := []string{"new-session", "-d", "-P", "-F", "#{pane_id}", "-s", sessionName, "-c", options.WorktreePath, "-n", windowName}
	newSession := runTmux(ctx, runner, newSessionArgs, false, false)
	if newSession.Err != nil || newSession.ExitCode != 0 {
		return 1, errors.New(run.FailureMessage("tmux", newSessionArgs, newSession))
	}
	paneID, err := parseNewSessionPane(newSession.Stdout)
	if err != nil {
		return 1, err
	}
	setupStatus := setup.CopyFiles(options.SetupPlan, options.Logger)
	if setup.SymlinkFiles(options.SetupPlan, options.Logger) != 0 {
		setupStatus = 1
	}
	if err := sendSetupCommands(ctx, runner, paneID, options.SetupPlan.PostSetup); err != nil {
		return 1, err
	}

	if options.Env["TMUX"] != "" {
		switchArgs := []string{"switch-client", "-t", sessionName}
		switched := runTmux(ctx, runner, switchArgs, false, false)
		if switched.Err != nil || switched.ExitCode != 0 {
			return 1, errors.New(run.FailureMessage("tmux", switchArgs, switched))
		}
		return setupStatus, nil
	}

	attachArgs := []string{"attach-session", "-t", sessionName}
	attach := runTmux(ctx, runner, attachArgs, true, true)
	if attach.Err != nil || attach.ExitCode != 0 {
		return 1, errors.New(run.FailureMessage("tmux", attachArgs, attach))
	}
	return setupStatus, nil
}

func parseNewSessionPane(output string) (string, error) {
	fields := strings.Fields(output)
	if len(fields) != 1 {
		return "", fmt.Errorf("tmux did not return a pane target")
	}
	return fields[0], nil
}

func sendSetupCommands(ctx context.Context, runner run.Runner, target string, commands []string) error {
	for _, command := range commands {
		args := []string{"send-keys", "-t", target, VisibleCommand(command), "C-m"}
		result := runTmux(ctx, runner, args, false, false)
		if result.Err != nil || result.ExitCode != 0 {
			return errors.New(run.FailureMessage("tmux", args, result))
		}
	}
	return nil
}

func ensureAvailable(ctx context.Context, runner run.Runner) error {
	result := runTmux(ctx, runner, []string{"-V"}, false, false)
	if result.Err != nil || result.ExitCode != 0 {
		return errors.New(run.FailureMessage("tmux", []string{"-V"}, result))
	}
	return nil
}

func runTmux(ctx context.Context, runner run.Runner, args []string, allowFailure bool, inherit bool) run.Result {
	result := runner.Run(ctx, "tmux", args, run.Options{Inherit: inherit})
	if !allowFailure && result.ExitCode != 0 && result.Err == nil {
		result.Err = fmt.Errorf("exit code %d", result.ExitCode)
	}
	return result
}

func singleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func pathNameComponent(value string) string {
	name := strings.ReplaceAll(value, ":", "-")
	name = unsafeNameChars.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-_.")
	return name
}
