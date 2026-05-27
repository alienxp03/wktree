package setup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alienxp03/wktree/internal/config"
	"github.com/alienxp03/wktree/internal/run"
)

type Plan struct {
	RepoRoot            string
	WorktreePath        string
	WorkspaceName       string
	Branch              string
	Copy                []string
	Symlink             []string
	RandomizePorts      []config.RandomizePort
	PreserveRandomPorts bool
	PostCreate          []string
	Context             Context
}

type Context struct {
	WorkspacePaths map[string]string
	PullRequest    *PullRequestContext
}

type PullRequestContext struct {
	Number  int
	URL     string
	HeadRef string
	HeadSHA string
}

type Logger struct {
	Stdout io.Writer
	Stderr io.Writer
	Prefix string
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

func NewPlan(repoRoot string, worktreePath string, workspaceName string, branch string, files config.Files, hooks config.Hooks, randomizePorts []config.RandomizePort, preserveRandomPorts bool, context Context) Plan {
	return Plan{
		RepoRoot:            repoRoot,
		WorktreePath:        worktreePath,
		WorkspaceName:       workspaceName,
		Branch:              branch,
		Copy:                append([]string(nil), files.Copy...),
		Symlink:             append([]string(nil), files.Symlink...),
		RandomizePorts:      append([]config.RandomizePort(nil), randomizePorts...),
		PreserveRandomPorts: preserveRandomPorts,
		PostCreate:          append([]string(nil), hooks.PostCreate...),
		Context:             context,
	}
}

func Run(ctx context.Context, plan Plan, logger Logger, shellRunner ShellRunner) int {
	existingRandomizeFiles := existingRandomizeDestinations(plan)
	copyStatus := CopyFiles(plan, logger)
	symlinkStatus := SymlinkFiles(plan, logger)
	randomizeStatus := randomizeEnvPorts(plan, logger, AllocateLocalPort, existingRandomizeFiles)
	contextStatus := 0
	if err := WriteContextEnv(plan); err != nil {
		logger.Warn("failed to write workspace env: %s", err)
		contextStatus = 1
	}
	commandStatus := RunPostCreate(ctx, plan, logger, shellRunner)
	if copyStatus == 0 && symlinkStatus == 0 && randomizeStatus == 0 && contextStatus == 0 && commandStatus == 0 {
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

type PortAllocator func() (int, error)

func RandomizeEnvPorts(plan Plan, logger Logger, allocate PortAllocator) int {
	return randomizeEnvPorts(plan, logger, allocate, nil)
}

func randomizeEnvPorts(plan Plan, logger Logger, allocate PortAllocator, existingFiles map[string]bool) int {
	if allocate == nil {
		allocate = AllocateLocalPort
	}
	status := 0
	used := map[int]bool{}
	for _, item := range plan.RandomizePorts {
		destinationPath := filepath.Join(plan.WorktreePath, item.File)
		if !isWithin(plan.WorktreePath, destinationPath) {
			logger.Warn("skipping unsafe randomize_ports path: %s", item.File)
			status = 1
			continue
		}
		source, err := os.ReadFile(destinationPath)
		if os.IsNotExist(err) {
			logger.Warn("randomize_ports file not found, skipping: %s", item.File)
			continue
		}
		if err != nil {
			logger.Warn("failed to read randomize_ports file %s: %s", item.File, err)
			status = 1
			continue
		}
		preserveExisting := plan.PreserveRandomPorts
		if existingFiles != nil {
			preserveExisting = preserveExisting && existingFiles[item.File]
		}
		updated, changed, err := renderRandomizedEnv(string(source), item.Vars, used, allocate, preserveExisting)
		if err != nil {
			logger.Warn("failed to randomize ports in %s: %s", item.File, err)
			status = 1
			continue
		}
		if !changed {
			continue
		}
		stat, err := os.Stat(destinationPath)
		if err != nil {
			logger.Warn("failed to stat randomize_ports file %s: %s", item.File, err)
			status = 1
			continue
		}
		if err := os.WriteFile(destinationPath, []byte(updated), stat.Mode().Perm()); err != nil {
			logger.Warn("failed to write randomized ports to %s: %s", item.File, err)
			status = 1
			continue
		}
		logger.Info("randomized ports in %s", item.File)
	}
	return status
}

func existingRandomizeDestinations(plan Plan) map[string]bool {
	existing := map[string]bool{}
	for _, item := range plan.RandomizePorts {
		destinationPath := filepath.Join(plan.WorktreePath, item.File)
		existing[item.File] = isWithin(plan.WorktreePath, destinationPath) && pathExists(destinationPath)
	}
	return existing
}

func AllocateLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("listener did not return a TCP address")
	}
	return address.Port, nil
}

func renderRandomizedEnv(source string, names []string, used map[int]bool, allocate PortAllocator, preserveExisting bool) (string, bool, error) {
	wanted := map[string]bool{}
	seen := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	lines := strings.SplitAfter(source, "\n")
	var output strings.Builder
	changed := false
	for _, line := range lines {
		name, prefix, ok := envAssignment(line)
		if !ok || !wanted[name] {
			output.WriteString(line)
			continue
		}
		seen[name] = true
		if port, ok := existingEnvPort(line); ok && preserveExisting {
			used[port] = true
			output.WriteString(line)
			continue
		}
		port, err := nextPort(used, allocate)
		if err != nil {
			return "", false, err
		}
		output.WriteString(prefix)
		output.WriteString(strconv.Itoa(port))
		if strings.HasSuffix(line, "\n") {
			output.WriteByte('\n')
		}
		changed = true
	}
	for _, name := range names {
		if seen[name] {
			continue
		}
		port, err := nextPort(used, allocate)
		if err != nil {
			return "", false, err
		}
		if output.Len() > 0 && !strings.HasSuffix(output.String(), "\n") {
			output.WriteByte('\n')
		}
		output.WriteString(name)
		output.WriteByte('=')
		output.WriteString(strconv.Itoa(port))
		output.WriteByte('\n')
		changed = true
	}
	return output.String(), changed, nil
}

func nextPort(used map[int]bool, allocate PortAllocator) (int, error) {
	for attempts := 0; attempts < 100; attempts++ {
		port, err := allocate()
		if err != nil {
			return 0, err
		}
		if port <= 0 || port > 65535 {
			return 0, fmt.Errorf("allocated invalid port %d", port)
		}
		if used[port] {
			continue
		}
		used[port] = true
		return port, nil
	}
	return 0, fmt.Errorf("could not allocate a unique port")
}

func envAssignment(line string) (string, string, bool) {
	trimmed := strings.TrimRight(line, "\r\n")
	leadingLength := len(trimmed) - len(strings.TrimLeft(trimmed, " \t"))
	leading := trimmed[:leadingLength]
	rest := trimmed[leadingLength:]
	export := ""
	if strings.HasPrefix(rest, "export ") {
		export = "export "
		rest = strings.TrimLeft(strings.TrimPrefix(rest, "export "), " \t")
	}
	key, _, ok := strings.Cut(rest, "=")
	if !ok {
		return "", "", false
	}
	name := strings.TrimSpace(key)
	if name == "" || strings.ContainsAny(name, " \t") {
		return "", "", false
	}
	return name, leading + export + name + "=", true
}

func existingEnvPort(line string) (int, bool) {
	trimmed := strings.TrimRight(line, "\r\n")
	_, value, ok := strings.Cut(trimmed, "=")
	if !ok {
		return 0, false
	}
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	port, err := strconv.Atoi(value)
	return port, err == nil && port > 0 && port <= 65535
}

func WriteContextEnv(plan Plan) error {
	envPath := ContextEnvPath(plan.WorktreePath)
	if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
		return err
	}
	content, err := RenderContextEnv(plan.Context)
	if err != nil {
		return err
	}
	return os.WriteFile(envPath, []byte(content), 0o600)
}

func RemoveContextEnv(worktreePath string) error {
	envPath := ContextEnvPath(worktreePath)
	if err := os.Remove(envPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func ContextEnvWorkspaceDirCount(worktreePath string) (int, error) {
	source, err := os.ReadFile(ContextEnvPath(worktreePath))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count := 0
	for _, line := range strings.Split(string(source), "\n") {
		if isContextWorkspaceDirLine(strings.TrimSpace(line)) {
			count++
		}
	}
	return count, nil
}

func ContextEnvPath(worktreePath string) string {
	return filepath.Join(worktreePath, ".wktree.env")
}

func RenderContextEnv(context Context) (string, error) {
	values := map[string]string{}
	namesByKey := map[string]string{}
	for name, path := range context.WorkspacePaths {
		key, err := config.WorkspaceDirEnvKey(name)
		if err != nil {
			return "", err
		}
		if previous, ok := namesByKey[key]; ok {
			return "", fmt.Errorf("workspace env var %s conflicts between %q and %q", key, previous, name)
		}
		namesByKey[key] = name
		absolute, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		values[key] = absolute
	}
	if context.PullRequest != nil {
		values["WKTREE_PR_NUMBER"] = strconv.Itoa(context.PullRequest.Number)
		values["WKTREE_PR_URL"] = context.PullRequest.URL
		values["WKTREE_PR_HEAD_REF"] = context.PullRequest.HeadRef
		values["WKTREE_PR_HEAD_SHA"] = context.PullRequest.HeadSHA
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var output strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&output, "export %s=%s\n", key, shellQuote(values[key]))
	}
	return output.String(), nil
}

func ReadPullRequestContext(worktreePath string) (PullRequestContext, bool, error) {
	source, err := os.ReadFile(ContextEnvPath(worktreePath))
	if os.IsNotExist(err) {
		return PullRequestContext{}, false, nil
	}
	if err != nil {
		return PullRequestContext{}, false, err
	}

	values := map[string]string{}
	for _, line := range strings.Split(string(source), "\n") {
		key, value, ok := exportedEnvValue(strings.TrimSpace(line))
		if ok {
			values[key] = value
		}
	}
	numberValue, ok := values["WKTREE_PR_NUMBER"]
	if !ok {
		return PullRequestContext{}, false, nil
	}
	number, err := strconv.Atoi(numberValue)
	if err != nil {
		return PullRequestContext{}, false, fmt.Errorf("invalid WKTREE_PR_NUMBER in %s: %w", ContextEnvPath(worktreePath), err)
	}
	return PullRequestContext{
		Number:  number,
		URL:     values["WKTREE_PR_URL"],
		HeadRef: values["WKTREE_PR_HEAD_REF"],
		HeadSHA: values["WKTREE_PR_HEAD_SHA"],
	}, true, nil
}

func RunPostCreate(ctx context.Context, plan Plan, logger Logger, shellRunner ShellRunner) int {
	if shellRunner == nil {
		shellRunner = DefaultShellRunner{}
	}
	for _, command := range plan.PostCreate {
		logger.Info("$ %s", command)
		result := shellRunner.RunShell(ctx, SourceEnvCommand(plan.WorktreePath, command), plan.WorktreePath, true)
		if result.ExitCode != 0 {
			detail := ""
			if result.Err != nil {
				detail = ": " + result.Err.Error()
			}
			logger.Warn("post create command failed (%d)%s: %s", result.ExitCode, detail, command)
			return 1
		}
	}
	return 0
}

func SourceEnvCommand(worktreePath string, command string) string {
	return ". " + shellQuote(ContextEnvPath(worktreePath)) + "; " + command
}

func JoinCommands(commands []string) string {
	return strings.Join(commands, " && ")
}

func isContextWorkspaceDirLine(line string) bool {
	const prefix = "export "
	if !strings.HasPrefix(line, prefix) {
		return false
	}
	key, value, ok := strings.Cut(strings.TrimPrefix(line, prefix), "=")
	if !ok || key == "WKTREE_WORKSPACE_DIR" {
		return false
	}
	if !strings.HasPrefix(key, "WKTREE_") || !strings.HasSuffix(key, "_DIR") {
		return false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(key, "WKTREE_"), "_DIR")
	if name == "" {
		return false
	}
	for _, char := range name {
		if (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\''
}

func exportedEnvValue(line string) (string, string, bool) {
	const prefix = "export "
	if !strings.HasPrefix(line, prefix) {
		return "", "", false
	}
	key, value, ok := strings.Cut(strings.TrimPrefix(line, prefix), "=")
	if !ok || strings.TrimSpace(key) == "" {
		return "", "", false
	}
	return strings.TrimSpace(key), shellUnquote(value), true
}

func shellUnquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		inner := value[1 : len(value)-1]
		return strings.ReplaceAll(inner, "'\\''", "'")
	}
	return strings.Trim(value, `"'`)
}

func (logger Logger) Info(format string, args ...any) {
	writer := logger.Stdout
	if writer == nil {
		writer = os.Stdout
	}
	fmt.Fprintf(writer, logger.prefix()+format+"\n", args...)
}

func (logger Logger) Warn(format string, args ...any) {
	writer := logger.Stderr
	if writer == nil {
		writer = os.Stderr
	}
	fmt.Fprintf(writer, "warning: "+logger.prefix()+format+"\n", args...)
}

func (logger Logger) prefix() string {
	if logger.Prefix == "" {
		return ""
	}
	return logger.Prefix + ": "
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

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
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
