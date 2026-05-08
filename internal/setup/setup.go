package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alienxp03/wktree/internal/config"
	"github.com/alienxp03/wktree/internal/run"
)

type Plan struct {
	RepoRoot     string   `json:"repoRoot"`
	WorktreePath string   `json:"worktreePath"`
	Copy         []string `json:"copy"`
	Symlink      []string `json:"symlink"`
	PostSetup    []string `json:"postSetup"`
}

type Logger struct {
	Stdout io.Writer
	Stderr io.Writer
}

type ShellRunner interface {
	RunShell(ctx context.Context, command string, cwd string, inherit bool) run.Result
}

type ShellRunnerFunc func(ctx context.Context, command string, cwd string, inherit bool) run.Result

func (fn ShellRunnerFunc) RunShell(ctx context.Context, command string, cwd string, inherit bool) run.Result {
	return fn(ctx, command, cwd, inherit)
}

type DefaultShellRunner struct{}

func (DefaultShellRunner) RunShell(ctx context.Context, command string, cwd string, inherit bool) run.Result {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Dir = cwd
	if inherit {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		err := cmd.Run()
		return shellResult(err, "", "")
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return shellResult(err, stdout.String(), stderr.String())
}

func NewPlan(repoRoot string, worktreePath string, setupConfig config.Config) Plan {
	return Plan{
		RepoRoot:     repoRoot,
		WorktreePath: worktreePath,
		Copy:         append([]string(nil), setupConfig.Copy...),
		Symlink:      append([]string(nil), setupConfig.Symlink...),
		PostSetup:    append([]string(nil), setupConfig.PostSetup...),
	}
}

func WritePlan(filePath string, plan Plan) error {
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filePath, data, 0o600)
}

func ReadPlan(filePath string) (Plan, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return Plan{}, err
	}
	var plan Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return Plan{}, err
	}
	if err := ValidatePlan(plan, filePath); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func ValidatePlan(plan Plan, filePath string) error {
	if strings.TrimSpace(plan.RepoRoot) == "" {
		return fmt.Errorf("invalid setup plan in %s: repoRoot is required", filePath)
	}
	if strings.TrimSpace(plan.WorktreePath) == "" {
		return fmt.Errorf("invalid setup plan in %s: worktreePath is required", filePath)
	}
	if err := validateRelativePaths(plan.Copy, "copy", filePath); err != nil {
		return err
	}
	if err := validateRelativePaths(plan.Symlink, "symlink", filePath); err != nil {
		return err
	}
	if err := validateNonEmpty(plan.PostSetup, "postSetup", filePath); err != nil {
		return err
	}
	return nil
}

func Run(ctx context.Context, plan Plan, logger Logger, shellRunner ShellRunner) int {
	copyStatus := CopyFiles(plan, logger)
	symlinkStatus := SymlinkFiles(plan, logger)
	commandStatus := RunPostSetup(ctx, plan, logger, shellRunner)
	if copyStatus == 0 && symlinkStatus == 0 && commandStatus == 0 {
		return 0
	}
	return 1
}

func CopyFiles(plan Plan, logger Logger) int {
	status := 0
	for _, relativePath := range plan.Copy {
		sourcePath := filepath.Join(plan.RepoRoot, relativePath)
		destinationPath := filepath.Join(plan.WorktreePath, relativePath)
		if !isWithin(plan.RepoRoot, sourcePath) || !isWithin(plan.WorktreePath, destinationPath) {
			logger.Warn("skipping unsafe copy path: %s", relativePath)
			status = 1
			continue
		}
		stat, err := os.Stat(sourcePath)
		if os.IsNotExist(err) {
			logger.Warn("copy source not found, skipping: %s", relativePath)
			continue
		}
		if err != nil {
			logger.Warn("failed to stat copy source %s: %s", relativePath, err)
			status = 1
			continue
		}
		if !stat.Mode().IsRegular() {
			logger.Warn("copy source is not a regular file, skipping: %s", relativePath)
			continue
		}
		if pathExists(destinationPath) {
			logger.Warn("copy destination already exists, skipping: %s", relativePath)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			logger.Warn("failed to create directory for %s: %s", relativePath, err)
			status = 1
			continue
		}
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			logger.Warn("failed to copy %s: %s", relativePath, err)
			status = 1
			continue
		}
		if err := os.WriteFile(destinationPath, data, stat.Mode().Perm()); err != nil {
			logger.Warn("failed to copy %s: %s", relativePath, err)
			status = 1
			continue
		}
		logger.Info("copied %s", relativePath)
	}
	return status
}

func SymlinkFiles(plan Plan, logger Logger) int {
	status := 0
	for _, relativePath := range plan.Symlink {
		sourcePath := filepath.Join(plan.RepoRoot, relativePath)
		destinationPath := filepath.Join(plan.WorktreePath, relativePath)
		if !isWithin(plan.RepoRoot, sourcePath) || !isWithin(plan.WorktreePath, destinationPath) {
			logger.Warn("skipping unsafe symlink path: %s", relativePath)
			status = 1
			continue
		}
		stat, err := os.Stat(sourcePath)
		if os.IsNotExist(err) {
			logger.Warn("symlink source not found, skipping: %s", relativePath)
			continue
		}
		if err != nil {
			logger.Warn("failed to stat symlink source %s: %s", relativePath, err)
			status = 1
			continue
		}
		if !stat.Mode().IsRegular() {
			logger.Warn("symlink source is not a regular file, skipping: %s", relativePath)
			continue
		}
		if pathExists(destinationPath) {
			logger.Warn("symlink destination already exists, skipping: %s", relativePath)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			logger.Warn("failed to create directory for %s: %s", relativePath, err)
			status = 1
			continue
		}
		if err := os.Symlink(sourcePath, destinationPath); err != nil {
			logger.Warn("failed to symlink %s: %s", relativePath, err)
			status = 1
			continue
		}
		logger.Info("symlinked %s", relativePath)
	}
	return status
}

func RunPostSetup(ctx context.Context, plan Plan, logger Logger, shellRunner ShellRunner) int {
	if shellRunner == nil {
		shellRunner = DefaultShellRunner{}
	}
	for _, command := range plan.PostSetup {
		logger.Info("$ %s", command)
		result := shellRunner.RunShell(ctx, command, plan.WorktreePath, true)
		if result.ExitCode != 0 {
			detail := ""
			if result.Err != nil {
				detail = ": " + result.Err.Error()
			}
			logger.Warn("post setup command failed (%d)%s: %s", result.ExitCode, detail, command)
			return 1
		}
	}
	return 0
}

func (logger Logger) Info(format string, args ...any) {
	writer := logger.Stdout
	if writer == nil {
		writer = os.Stdout
	}
	fmt.Fprintf(writer, format+"\n", args...)
}

func (logger Logger) Warn(format string, args ...any) {
	writer := logger.Stderr
	if writer == nil {
		writer = os.Stderr
	}
	fmt.Fprintf(writer, "warning: "+format+"\n", args...)
}

func validateRelativePaths(values []string, key string, filePath string) error {
	if err := validateNonEmpty(values, key, filePath); err != nil {
		return err
	}
	for index, value := range values {
		label := fmt.Sprintf("%s[%d]", key, index)
		if filepath.IsAbs(value) || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("invalid setup plan in %s: %s must be a safe relative path", filePath, label)
		}
		for _, segment := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
			if segment == ".." {
				return fmt.Errorf("invalid setup plan in %s: %s must be a safe relative path", filePath, label)
			}
		}
	}
	return nil
}

func validateNonEmpty(values []string, key string, filePath string) error {
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("invalid setup plan in %s: %s[%d] must be a non-empty string", filePath, key, index)
		}
	}
	return nil
}

func isWithin(root string, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && (relative == "." || (!strings.HasPrefix(relative, "..") && !filepath.IsAbs(relative)))
}

func pathExists(candidate string) bool {
	_, err := os.Lstat(candidate)
	return err == nil || !os.IsNotExist(err)
}

func shellResult(err error, stdout string, stderr string) run.Result {
	if err == nil {
		return run.Result{Stdout: stdout, Stderr: stderr, ExitCode: 0}
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return run.Result{Stdout: stdout, Stderr: stderr, ExitCode: exitErr.ExitCode(), Err: err}
	}
	return run.Result{Stdout: stdout, Stderr: stderr, ExitCode: 1, Err: err}
}
