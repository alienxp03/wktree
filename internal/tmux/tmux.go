package tmux

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/alienxp03/wktree/internal/run"
	"github.com/alienxp03/wktree/internal/setup"
)

const (
	ModeWindow  = "window"
	ModeSession = "session"
)

var unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

type LayoutOptions struct {
	Mode        string
	SessionName string
	Windows     []Window
	Env         map[string]string
	Runner      run.Runner
}

type Window struct {
	Name         string
	WorktreePath string
	Commands     []PaneCommand
}

type PaneCommand struct {
	Command    string
	Commands   []string
	Split      string
	Focus      bool
	Zoom       bool
	Size       string
	Percentage int
}

type KillOptions struct {
	Mode        string
	SessionName string
	WindowNames []string
	KillSession bool
	Env         map[string]string
	Runner      run.Runner
}

func OpenLayout(ctx context.Context, options LayoutOptions) (int, error) {
	runner := options.Runner
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	if err := Available(ctx, runner); err != nil {
		return 1, err
	}
	switch options.Mode {
	case ModeSession:
		return openSessionLayout(ctx, runner, options)
	case ModeWindow, "":
		return openWindowLayout(ctx, runner, options)
	default:
		return 1, fmt.Errorf("unsupported tmux mode: %s", options.Mode)
	}
}

func KillLayout(ctx context.Context, options KillOptions) error {
	runner := options.Runner
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	if err := Available(ctx, runner); err != nil {
		return nil
	}
	switch options.Mode {
	case ModeSession:
		if options.KillSession {
			return killSession(ctx, runner, options.SessionName)
		}
		for _, windowName := range options.WindowNames {
			if err := killWindow(ctx, runner, options.SessionName+":"+windowName); err != nil {
				return err
			}
		}
		return nil
	case ModeWindow, "":
		if options.Env["TMUX"] == "" {
			return fmt.Errorf("tmux window mode requires running inside tmux")
		}
		for _, windowName := range options.WindowNames {
			if err := killWindow(ctx, runner, windowName); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported tmux mode: %s", options.Mode)
	}
}

func Available(ctx context.Context, runner run.Runner) error {
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	return ensureAvailable(ctx, runner)
}

func KillSessionForWorktree(ctx context.Context, worktreePath string, runner run.Runner) error {
	if runner == nil {
		runner = run.DefaultRunner{}
	}
	if err := Available(ctx, runner); err != nil {
		return nil
	}
	return killSession(ctx, runner, SessionName(worktreePath))
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

func TargetName(value string) string {
	name := pathNameComponent(value)
	if name == "" {
		return "wktree"
	}
	return name
}

func VisibleCommand(worktreePath string, command string) string {
	return strings.Join([]string{
		SourceShellStartupCommand(),
		"printf '%s\\n' " + singleQuote("$ "+command),
		setup.SourceEnvCommand(worktreePath, "eval "+singleQuote(command)),
		"wktree_status=$?",
		"if [ \"$wktree_status\" -ne 0 ]; then printf '%s\\n' " + singleQuote("warning: pane command failed: "+command) + " >&2; fi",
		"exec \"${SHELL:-/bin/sh}\" -i",
	}, "; ")
}

func SourceShellStartupCommand() string {
	return strings.Join([]string{
		"if [ -n \"${ZSH_VERSION:-}\" ] && [ -r \"${ZDOTDIR:-$HOME}/.zshrc\" ]; then . \"${ZDOTDIR:-$HOME}/.zshrc\" >/dev/null 2>&1; fi",
		"if [ -n \"${BASH_VERSION:-}\" ] && [ -r \"$HOME/.bashrc\" ]; then shopt -s expand_aliases 2>/dev/null || true; . \"$HOME/.bashrc\" >/dev/null 2>&1; fi",
	}, "; ")
}

func PaneShellCommand(worktreePath string, command string) string {
	return "exec \"${SHELL:-/bin/sh}\" -fc " + singleQuote(VisibleCommand(worktreePath, command))
}

func openWindowLayout(ctx context.Context, runner run.Runner, options LayoutOptions) (int, error) {
	if options.Env["TMUX"] == "" {
		return 1, fmt.Errorf("tmux window mode requires running inside tmux")
	}
	var firstWindow string
	for _, window := range options.Windows {
		if firstWindow == "" {
			firstWindow = window.Name
		}
		if ok, err := hasWindow(ctx, runner, "", window.Name); err != nil {
			return 1, err
		} else if ok {
			continue
		}
		newWindowArgs := []string{"new-window", "-P", "-F", "#{pane_id}", "-c", window.WorktreePath, "-n", window.Name}
		newWindow := runTmux(ctx, runner, newWindowArgs, false, false)
		if newWindow.Err != nil || newWindow.ExitCode != 0 {
			return 1, errors.New(run.FailureMessage("tmux", newWindowArgs, newWindow))
		}
		paneID, err := parsePaneID(newWindow.Stdout)
		if err != nil {
			return 1, err
		}
		if err := buildPanes(ctx, runner, paneID, window); err != nil {
			return 1, err
		}
	}
	if firstWindow != "" {
		return 0, selectWindow(ctx, runner, firstWindow)
	}
	return 0, nil
}

func openSessionLayout(ctx context.Context, runner run.Runner, options LayoutOptions) (int, error) {
	if ok, err := hasSession(ctx, runner, options.SessionName); err != nil {
		return 1, err
	} else if ok {
		return switchOrAttach(ctx, runner, options.SessionName, options.Env)
	}
	if len(options.Windows) == 0 {
		return 0, nil
	}

	first := options.Windows[0]
	newSessionArgs := []string{"new-session", "-d", "-P", "-F", "#{pane_id}", "-s", options.SessionName, "-c", first.WorktreePath, "-n", first.Name}
	newSession := runTmux(ctx, runner, newSessionArgs, false, false)
	if newSession.Err != nil || newSession.ExitCode != 0 {
		return 1, errors.New(run.FailureMessage("tmux", newSessionArgs, newSession))
	}
	paneID, err := parsePaneID(newSession.Stdout)
	if err != nil {
		return 1, err
	}
	if err := buildPanes(ctx, runner, paneID, first); err != nil {
		return 1, err
	}

	for _, window := range options.Windows[1:] {
		newWindowArgs := []string{"new-window", "-P", "-F", "#{pane_id}", "-t", options.SessionName + ":", "-c", window.WorktreePath, "-n", window.Name}
		newWindow := runTmux(ctx, runner, newWindowArgs, false, false)
		if newWindow.Err != nil || newWindow.ExitCode != 0 {
			return 1, errors.New(run.FailureMessage("tmux", newWindowArgs, newWindow))
		}
		paneID, err := parsePaneID(newWindow.Stdout)
		if err != nil {
			return 1, err
		}
		if err := buildPanes(ctx, runner, paneID, window); err != nil {
			return 1, err
		}
	}
	return switchOrAttach(ctx, runner, options.SessionName, options.Env)
}

func buildPanes(ctx context.Context, runner run.Runner, firstPaneID string, window Window) error {
	focusPane := firstPaneID
	zoomPane := ""
	previousPaneID := firstPaneID
	for index, command := range window.Commands {
		paneID := firstPaneID
		if index > 0 {
			splitArgs := []string{"split-window", "-P", "-F", "#{pane_id}", "-c", window.WorktreePath, "-t", previousPaneID}
			split := command.Split
			if split == "" {
				split = "horizontal"
			}
			if split == "horizontal" {
				splitArgs = append(splitArgs, "-h")
			} else if split == "vertical" {
				splitArgs = append(splitArgs, "-v")
			}
			if command.Size != "" {
				splitArgs = append(splitArgs, "-l", command.Size)
			}
			if command.Percentage > 0 {
				splitArgs = append(splitArgs, "-p", strconv.Itoa(command.Percentage))
			}
			splitResult := runTmux(ctx, runner, splitArgs, false, false)
			if splitResult.Err != nil || splitResult.ExitCode != 0 {
				return errors.New(run.FailureMessage("tmux", splitArgs, splitResult))
			}
			var err error
			paneID, err = parsePaneID(splitResult.Stdout)
			if err != nil {
				return err
			}
		}
		previousPaneID = paneID
		commandText := paneCommandText(command)
		if commandText != "" {
			respawnArgs := []string{"respawn-pane", "-k", "-c", window.WorktreePath, "-t", paneID, PaneShellCommand(window.WorktreePath, commandText)}
			respawned := runTmux(ctx, runner, respawnArgs, false, false)
			if respawned.Err != nil || respawned.ExitCode != 0 {
				return errors.New(run.FailureMessage("tmux", respawnArgs, respawned))
			}
		}
		if command.Focus || command.Zoom {
			focusPane = paneID
		}
		if command.Zoom {
			zoomPane = paneID
		}
	}
	if focusPane != "" {
		if err := selectPane(ctx, runner, focusPane); err != nil {
			return err
		}
	}
	if zoomPane != "" {
		return zoom(ctx, runner, zoomPane)
	}
	return nil
}

func paneCommandText(command PaneCommand) string {
	if len(command.Commands) > 0 {
		return setup.JoinCommands(command.Commands)
	}
	return command.Command
}

func switchOrAttach(ctx context.Context, runner run.Runner, sessionName string, env map[string]string) (int, error) {
	if env["TMUX"] != "" {
		switchArgs := []string{"switch-client", "-t", sessionName}
		switched := runTmux(ctx, runner, switchArgs, false, false)
		if switched.Err != nil || switched.ExitCode != 0 {
			return 1, errors.New(run.FailureMessage("tmux", switchArgs, switched))
		}
		return 0, nil
	}
	attachArgs := []string{"attach-session", "-t", sessionName}
	attach := runTmux(ctx, runner, attachArgs, true, true)
	if attach.Err != nil || attach.ExitCode != 0 {
		return 1, errors.New(run.FailureMessage("tmux", attachArgs, attach))
	}
	return 0, nil
}

func hasSession(ctx context.Context, runner run.Runner, sessionName string) (bool, error) {
	args := []string{"has-session", "-t", sessionName}
	result := runTmux(ctx, runner, args, true, false)
	if result.ExitCode == 0 {
		return true, nil
	}
	if result.ExitCode == 1 {
		return false, nil
	}
	return false, errors.New(run.FailureMessage("tmux", args, result))
}

func hasWindow(ctx context.Context, runner run.Runner, sessionName string, windowName string) (bool, error) {
	args := []string{"list-windows", "-F", "#{window_name}"}
	if sessionName != "" {
		args = []string{"list-windows", "-t", sessionName, "-F", "#{window_name}"}
	}
	result := runTmux(ctx, runner, args, true, false)
	if result.ExitCode != 0 {
		return false, errors.New(run.FailureMessage("tmux", args, result))
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		if strings.TrimSpace(line) == windowName {
			return true, nil
		}
	}
	return false, nil
}

func killSession(ctx context.Context, runner run.Runner, sessionName string) error {
	ok, err := hasSession(ctx, runner, sessionName)
	if err != nil || !ok {
		return err
	}
	killArgs := []string{"kill-session", "-t", sessionName}
	killed := runTmux(ctx, runner, killArgs, false, false)
	if killed.Err != nil || killed.ExitCode != 0 {
		return errors.New(run.FailureMessage("tmux", killArgs, killed))
	}
	return nil
}

func killWindow(ctx context.Context, runner run.Runner, target string) error {
	killArgs := []string{"kill-window", "-t", target}
	killed := runTmux(ctx, runner, killArgs, true, false)
	if killed.ExitCode == 1 {
		return nil
	}
	if killed.Err != nil || killed.ExitCode != 0 {
		return errors.New(run.FailureMessage("tmux", killArgs, killed))
	}
	return nil
}

func selectWindow(ctx context.Context, runner run.Runner, target string) error {
	args := []string{"select-window", "-t", target}
	result := runTmux(ctx, runner, args, false, false)
	if result.Err != nil || result.ExitCode != 0 {
		return errors.New(run.FailureMessage("tmux", args, result))
	}
	return nil
}

func selectPane(ctx context.Context, runner run.Runner, target string) error {
	args := []string{"select-pane", "-t", target}
	result := runTmux(ctx, runner, args, false, false)
	if result.Err != nil || result.ExitCode != 0 {
		return errors.New(run.FailureMessage("tmux", args, result))
	}
	return nil
}

func zoom(ctx context.Context, runner run.Runner, target string) error {
	args := []string{"resize-pane", "-Z", "-t", target}
	result := runTmux(ctx, runner, args, false, false)
	if result.Err != nil || result.ExitCode != 0 {
		return errors.New(run.FailureMessage("tmux", args, result))
	}
	return nil
}

func parsePaneID(output string) (string, error) {
	fields := strings.Fields(output)
	if len(fields) != 1 {
		return "", fmt.Errorf("tmux did not return a pane target")
	}
	return fields[0], nil
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
