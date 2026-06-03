package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectPath(t *testing.T) {
	if ProjectPath("/repo") != filepath.Join("/repo", ".wktree.yaml") {
		t.Fatalf("project path = %q", ProjectPath("/repo"))
	}
}

func TestProjectTemplate(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, ".wktree.yaml")
	write(t, configPath, ProjectTemplate())

	loaded, err := LoadFile(configPath, filepath.Join(root, "home"))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.WorktreeDir != "" || loaded.Tmux.Mode != "window" || loaded.WorkspaceMode != "single" {
		t.Fatalf("config = %#v", loaded)
	}
	if len(loaded.Workspaces) != 1 || loaded.Workspaces[0].Name != "window_name" || loaded.Workspaces[0].Repo != "." {
		t.Fatalf("workspaces = %#v", loaded.Workspaces)
	}
	template := ProjectTemplate()
	for _, want := range []string{"# worktree_dir: ~/workspace/worktrees", "# tmux:", "#   mode: window", "#   session_name: \"${repo}/${branch}\"", "# workspace_mode: single", "# files:", "# hooks:", "#   post_create:", "# randomize_ports:", "#       - PORT", "# set_env:", "#       API_URL:", "# open:", "# panes:"} {
		if !strings.Contains(template, want) {
			t.Fatalf("template missing %q:\n%s", want, template)
		}
	}
}

func TestWriteProjectTemplateRefusesExistingConfig(t *testing.T) {
	root := t.TempDir()
	configPath, err := WriteProjectTemplate(root)
	if err != nil {
		t.Fatal(err)
	}
	if configPath != ProjectPath(root) {
		t.Fatalf("config path = %q", configPath)
	}
	if _, err := WriteProjectTemplate(root); err == nil || !strings.Contains(err.Error(), "config already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestFindProjectPathUsesNearestConfigWithinRepo(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "projects", "project_a")
	startDir := filepath.Join(projectRoot, "src")
	must(t, os.MkdirAll(startDir, 0o755))
	rootConfig := filepath.Join(root, ".wktree.yaml")
	projectConfig := filepath.Join(projectRoot, ".wktree.yaml")
	write(t, rootConfig, "workspaces:\n  - name: root\n")
	write(t, projectConfig, "workspaces:\n  - name: project_a\n")

	got, found, err := FindProjectPath(startDir, root)
	if err != nil {
		t.Fatal(err)
	}
	if !found || got != projectConfig {
		t.Fatalf("project config = %q found=%v", got, found)
	}

	must(t, os.Remove(projectConfig))
	got, found, err = FindProjectPath(startDir, root)
	if err != nil {
		t.Fatal(err)
	}
	if !found || got != rootConfig {
		t.Fatalf("root config = %q found=%v", got, found)
	}

	must(t, os.Remove(rootConfig))
	got, found, err = FindProjectPath(startDir, root)
	if err != nil {
		t.Fatal(err)
	}
	if found || got != rootConfig {
		t.Fatalf("missing config = %q found=%v", got, found)
	}
}

func TestLoadProjectConfig(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	must(t, os.MkdirAll(sourceRoot, 0o755))
	write(t, filepath.Join(sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"worktree_dir: ~/worktree",
		"tmux:",
		"  mode: session",
		"workspace_mode: all",
		"defaults:",
		"  files:",
		"    copy:",
		"      - .env",
		"workspaces:",
		"  - name: backend",
		"    panes:",
		"      - command: nvim",
		"        focus: true",
		"      - commands:",
		"          - pnpm install",
		"          - pnpm run dev",
		"        split: horizontal",
		"  - name: frontend",
		"    repo: ../frontend",
		"    files:",
		"      symlink:",
		"        - AGENTS.override.md",
		"    hooks:",
		"      post_create:",
		"        - pnpm install",
		"    randomize_ports:",
		"      - file: .env.local",
		"        vars:",
		"          - PORT",
		"          - APP_PORT",
		"    set_env:",
		"      - file: .env.local",
		"        vars:",
		"          API_URL: http://localhost:${backend:PORT}/api",
		"    open:",
		"      - http://localhost:${backend:PORT}/api",
		"",
	}, "\n"))

	config, err := LoadProject(sourceRoot, filepath.Join(root, "home"))
	if err != nil {
		t.Fatal(err)
	}
	if config.WorktreeDir != "~/worktree" || config.Tmux.Mode != "session" || config.WorkspaceMode != "all" {
		t.Fatalf("config = %#v", config)
	}
	if len(config.Workspaces) != 2 || config.Workspaces[0].Name != "backend" || config.Workspaces[1].Repo != "../frontend" {
		t.Fatalf("workspaces = %#v", config.Workspaces)
	}
	panes := WorkspacePanes(config.Workspaces[0])
	if len(panes) != 2 || panes[0].Command != "nvim" || panes[1].Split != "horizontal" {
		t.Fatalf("panes = %#v", panes)
	}
	if !config.HasSetup() {
		t.Fatal("expected setup")
	}
	files := WorkspaceFiles(config, config.Workspaces[1])
	assertSlice(t, files.Copy, []string{".env"})
	assertSlice(t, files.Symlink, []string{"AGENTS.override.md"})
	hooks := WorkspaceHooks(config, config.Workspaces[1])
	assertSlice(t, hooks.PostCreate, []string{"pnpm install"})
	randomizePorts := config.Workspaces[1].RandomizePorts
	if len(randomizePorts) != 1 || randomizePorts[0].File != ".env.local" {
		t.Fatalf("randomize ports = %#v", randomizePorts)
	}
	assertSlice(t, randomizePorts[0].Vars, []string{"PORT", "APP_PORT"})
	setEnv := config.Workspaces[1].SetEnv
	if len(setEnv) != 1 || setEnv[0].File != ".env.local" || setEnv[0].Vars["API_URL"] != "http://localhost:${backend:PORT}/api" {
		t.Fatalf("set env = %#v", setEnv)
	}
	assertSlice(t, config.Workspaces[1].Open, []string{"http://localhost:${backend:PORT}/api"})
}

func TestLoadProjectConfigLegacyAliases(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	must(t, os.MkdirAll(sourceRoot, 0o755))
	write(t, filepath.Join(sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"files:",
		"  copy:",
		"    - .env",
		"hooks:",
		"  post_create:",
		"    - mise use",
		"workspaces:",
		"  - name: backend",
		"    commands:",
		"      - command: nvim",
		"",
	}, "\n"))

	config, err := LoadProject(sourceRoot, filepath.Join(root, "home"))
	if err != nil {
		t.Fatal(err)
	}
	panes := WorkspacePanes(config.Workspaces[0])
	if len(panes) != 1 || panes[0].Command != "nvim" {
		t.Fatalf("panes = %#v", panes)
	}
	files := WorkspaceFiles(config, config.Workspaces[0])
	assertSlice(t, files.Copy, []string{".env"})
	hooks := WorkspaceHooks(config, config.Workspaces[0])
	assertSlice(t, hooks.PostCreate, []string{"mise use"})
}

func TestLoadFileMissingAndEmpty(t *testing.T) {
	root := t.TempDir()
	config, err := LoadFile(filepath.Join(root, "missing.yaml"), filepath.Join(root, "home"))
	if err != nil {
		t.Fatal(err)
	}
	if config.HasSetup() {
		t.Fatal("missing config should be empty")
	}

	empty := filepath.Join(root, "empty.yaml")
	write(t, empty, "\n")
	config, err = LoadFile(empty, filepath.Join(root, "home"))
	if err != nil {
		t.Fatal(err)
	}
	if config.HasSetup() {
		t.Fatal("empty config should be empty")
	}
}

func TestLoadFileRejectsInvalidConfig(t *testing.T) {
	root := t.TempDir()
	invalidYAML := filepath.Join(root, "invalid.yaml")
	legacyCopy := filepath.Join(root, "legacy-copy.yaml")
	legacyMode := filepath.Join(root, "legacy-mode.yaml")
	legacyDefaultWorkspaces := filepath.Join(root, "legacy-default-workspaces.yaml")
	unsupported := filepath.Join(root, "unsupported.yaml")
	duplicateWorkspace := filepath.Join(root, "duplicate-workspace.yaml")
	duplicateWorkspaceEnv := filepath.Join(root, "duplicate-workspace-env.yaml")
	emptyWorkspaceEnv := filepath.Join(root, "empty-workspace-env.yaml")
	missingCommand := filepath.Join(root, "missing-command.yaml")
	bothCommandShapes := filepath.Join(root, "both-command-shapes.yaml")
	bothPaneKeys := filepath.Join(root, "both-pane-keys.yaml")
	bothDefaultFiles := filepath.Join(root, "both-default-files.yaml")
	defaultHooks := filepath.Join(root, "default-hooks.yaml")
	badTmuxMode := filepath.Join(root, "bad-tmux-mode.yaml")
	badTmuxSessionNameReference := filepath.Join(root, "bad-tmux-session-name-reference.yaml")
	badTmuxSessionNameSyntax := filepath.Join(root, "bad-tmux-session-name-syntax.yaml")
	badTmuxKey := filepath.Join(root, "bad-tmux-key.yaml")
	badWorkspaceMode := filepath.Join(root, "bad-workspace-mode.yaml")
	badSplit := filepath.Join(root, "bad-split.yaml")
	unsafeCopy := filepath.Join(root, "unsafe-copy.yaml")
	unsafeRandomizePortFile := filepath.Join(root, "unsafe-randomize-port-file.yaml")
	emptyRandomizePortVars := filepath.Join(root, "empty-randomize-port-vars.yaml")
	badRandomizePortVar := filepath.Join(root, "bad-randomize-port-var.yaml")
	duplicateRandomizePortVar := filepath.Join(root, "duplicate-randomize-port-var.yaml")
	unsafeSetEnvFile := filepath.Join(root, "unsafe-set-env-file.yaml")
	emptySetEnvVars := filepath.Join(root, "empty-set-env-vars.yaml")
	badSetEnvVar := filepath.Join(root, "bad-set-env-var.yaml")
	emptySetEnvTemplate := filepath.Join(root, "empty-set-env-template.yaml")
	emptyOpen := filepath.Join(root, "empty-open.yaml")

	write(t, invalidYAML, "workspaces: [\n")
	write(t, legacyCopy, "copy:\n  - .env\n")
	write(t, legacyMode, "mode: session\n")
	write(t, legacyDefaultWorkspaces, "default_workspaces: true\n")
	write(t, unsupported, "commands:\n  - pnpm install\n")
	write(t, duplicateWorkspace, "workspaces:\n  - name: app\n  - name: app\n")
	write(t, duplicateWorkspaceEnv, "workspaces:\n  - name: front-end\n  - name: front end\n")
	write(t, emptyWorkspaceEnv, "workspaces:\n  - name: '---'\n")
	write(t, missingCommand, "workspaces:\n  - name: app\n    panes:\n      - split: horizontal\n")
	write(t, bothCommandShapes, "workspaces:\n  - name: app\n    panes:\n      - command: nvim\n        commands:\n          - pnpm install\n")
	write(t, bothPaneKeys, "workspaces:\n  - name: app\n    panes:\n      - command: nvim\n    commands:\n      - command: codex\n")
	write(t, bothDefaultFiles, "defaults:\n  files:\n    copy:\n      - .env\nfiles:\n  copy:\n    - .env.local\n")
	write(t, defaultHooks, "defaults:\n  hooks:\n    post_create:\n      - pnpm install\n")
	write(t, badTmuxMode, "tmux:\n  mode: pane\n")
	write(t, badTmuxSessionNameReference, "tmux:\n  session_name: ${workspace}\n")
	write(t, badTmuxSessionNameSyntax, "tmux:\n  session_name: ${branch\n")
	write(t, badTmuxKey, "tmux:\n  name: app\n")
	write(t, badWorkspaceMode, "workspace_mode: many\n")
	write(t, badSplit, "workspaces:\n  - name: app\n    panes:\n      - command: nvim\n        split: diagonal\n")
	write(t, unsafeCopy, "defaults:\n  files:\n    copy:\n      - ../.env\n")
	write(t, unsafeRandomizePortFile, "workspaces:\n  - name: app\n    randomize_ports:\n      - file: ../.env\n        vars:\n          - PORT\n")
	write(t, emptyRandomizePortVars, "workspaces:\n  - name: app\n    randomize_ports:\n      - file: .env\n")
	write(t, badRandomizePortVar, "workspaces:\n  - name: app\n    randomize_ports:\n      - file: .env\n        vars:\n          - APP-PORT\n")
	write(t, duplicateRandomizePortVar, "workspaces:\n  - name: app\n    randomize_ports:\n      - file: .env\n        vars:\n          - PORT\n          - PORT\n")
	write(t, unsafeSetEnvFile, "workspaces:\n  - name: app\n    set_env:\n      - file: ../.env\n        vars:\n          URL: http://localhost:3000\n")
	write(t, emptySetEnvVars, "workspaces:\n  - name: app\n    set_env:\n      - file: .env\n")
	write(t, badSetEnvVar, "workspaces:\n  - name: app\n    set_env:\n      - file: .env\n        vars:\n          APP-URL: http://localhost:3000\n")
	write(t, emptySetEnvTemplate, "workspaces:\n  - name: app\n    set_env:\n      - file: .env\n        vars:\n          URL: ''\n")
	write(t, emptyOpen, "workspaces:\n  - name: app\n    open:\n      - ''\n")

	loadErrorContains(t, invalidYAML, "invalid YAML")
	loadErrorContains(t, legacyCopy, "legacy key")
	loadErrorContains(t, legacyMode, "legacy key")
	loadErrorContains(t, legacyDefaultWorkspaces, "legacy key")
	loadErrorContains(t, unsupported, "unsupported key")
	loadErrorContains(t, duplicateWorkspace, "duplicate workspace name")
	loadErrorContains(t, duplicateWorkspaceEnv, "workspace env var")
	loadErrorContains(t, emptyWorkspaceEnv, "at least one letter or digit")
	loadErrorContains(t, missingCommand, "must define command or commands")
	loadErrorContains(t, bothCommandShapes, "not both")
	loadErrorContains(t, bothPaneKeys, "panes or commands")
	loadErrorContains(t, bothDefaultFiles, "defaults.files or files")
	loadErrorContains(t, defaultHooks, "defaults.hooks is not supported")
	loadErrorContains(t, badTmuxMode, "tmux.mode")
	loadErrorContains(t, badTmuxSessionNameReference, "tmux.session_name")
	loadErrorContains(t, badTmuxSessionNameSyntax, "tmux.session_name")
	loadErrorContains(t, badTmuxKey, "unsupported tmux key")
	loadErrorContains(t, badWorkspaceMode, "workspace_mode")
	loadErrorContains(t, badSplit, "split")
	loadErrorContains(t, unsafeCopy, `cannot contain ".."`)
	loadErrorContains(t, unsafeRandomizePortFile, `cannot contain ".."`)
	loadErrorContains(t, emptyRandomizePortVars, "must define at least one variable")
	loadErrorContains(t, badRandomizePortVar, "valid env var name")
	loadErrorContains(t, duplicateRandomizePortVar, "duplicate randomize port var")
	loadErrorContains(t, unsafeSetEnvFile, `cannot contain ".."`)
	loadErrorContains(t, emptySetEnvVars, "must define at least one variable")
	loadErrorContains(t, badSetEnvVar, "valid env var name")
	if _, err := LoadFile(emptySetEnvTemplate, filepath.Join(root, "home")); err != nil {
		t.Fatalf("empty set_env value should be valid: %v", err)
	}
	loadErrorContains(t, emptyOpen, "must be a non-empty string")
}

func TestWorkspaceDirEnvKey(t *testing.T) {
	got, err := WorkspaceDirEnvKey("front-end app")
	if err != nil {
		t.Fatal(err)
	}
	if got != "WKTREE_FRONT_END_APP_DIR" {
		t.Fatalf("key = %q", got)
	}
	got, err = WorkspaceDirEnvKey("123")
	if err != nil {
		t.Fatal(err)
	}
	if got != "WKTREE_123_DIR" {
		t.Fatalf("numeric key = %q", got)
	}
	if _, err := WorkspaceDirEnvKey("---"); err == nil {
		t.Fatal("expected empty sanitized name error")
	}
}

func TestExpandConfiguredPath(t *testing.T) {
	root := t.TempDir()
	got, err := ExpandConfiguredPath("~/repo", filepath.Join(root, "home"), root)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "home", "repo") {
		t.Fatalf("expanded = %q", got)
	}

	got, err = ExpandConfiguredPath("../repo", filepath.Join(root, "home"), filepath.Join(root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(root, "repo") {
		t.Fatalf("relative expanded = %q", got)
	}
}

func assertSlice(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}

func loadErrorContains(t *testing.T, filePath string, want string) {
	t.Helper()
	_, err := LoadFile(filePath, filepath.Join(filepath.Dir(filePath), "home"))
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want containing %q", err, want)
	}
}

func write(t *testing.T, filePath string, content string) {
	t.Helper()
	must(t, os.WriteFile(filePath, []byte(content), 0o644))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
