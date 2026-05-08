package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPaths(t *testing.T) {
	got, err := GlobalPath(LoadOptions{Env: map[string]string{"XDG_CONFIG_HOME": "/tmp/xdg"}, HomeDir: "/Users/stan"})
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join("/tmp/xdg", "wktree", "config.yaml") {
		t.Fatalf("global path = %q", got)
	}

	got, err = GlobalPath(LoadOptions{Env: map[string]string{}, HomeDir: "/Users/stan"})
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join("/Users/stan", ".config", "wktree", "config.yaml") {
		t.Fatalf("fallback global path = %q", got)
	}
	if ProjectPath("/repo") != filepath.Join("/repo", ".wktree.yaml") {
		t.Fatalf("project path = %q", ProjectPath("/repo"))
	}
}

func TestLoadMerged(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "source")
	xdgHome := filepath.Join(root, "xdg")
	must(t, os.MkdirAll(filepath.Join(xdgHome, "wktree"), 0o755))
	must(t, os.MkdirAll(sourceRoot, 0o755))
	write(t, filepath.Join(xdgHome, "wktree", "config.yaml"), strings.Join([]string{
		"copy:",
		"  - .env",
		"symlink:",
		"  - .mcp.json",
		"postSetup:",
		"  - gcloud auth application-default login",
		"",
	}, "\n"))
	write(t, filepath.Join(sourceRoot, ".wktree.yaml"), strings.Join([]string{
		"copy:",
		"  - .env.local",
		"symlink:",
		"  - .tool-versions",
		"postSetup:",
		"  - pnpm install",
		"",
	}, "\n"))

	config, err := LoadMerged(sourceRoot, LoadOptions{Env: map[string]string{"XDG_CONFIG_HOME": xdgHome}, HomeDir: filepath.Join(root, "home")})
	if err != nil {
		t.Fatal(err)
	}
	assertSlice(t, config.Copy, []string{".env", ".env.local"})
	assertSlice(t, config.Symlink, []string{".mcp.json", ".tool-versions"})
	assertSlice(t, config.PostSetup, []string{"gcloud auth application-default login", "pnpm install"})
	if !HasSetup(config) {
		t.Fatal("expected setup")
	}
}

func TestLoadFileMissingAndEmpty(t *testing.T) {
	root := t.TempDir()
	config, err := LoadFile(filepath.Join(root, "missing.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if HasSetup(config) {
		t.Fatal("missing config should be empty")
	}

	empty := filepath.Join(root, "empty.yaml")
	write(t, empty, "\n")
	config, err = LoadFile(empty)
	if err != nil {
		t.Fatal(err)
	}
	if HasSetup(config) {
		t.Fatal("empty config should be empty")
	}
}

func TestLoadFileRejectsInvalidConfig(t *testing.T) {
	root := t.TempDir()
	invalidYAML := filepath.Join(root, "invalid.yaml")
	unsafeCopy := filepath.Join(root, "unsafe-copy.yaml")
	unsafeSymlink := filepath.Join(root, "unsafe-symlink.yaml")
	unsupported := filepath.Join(root, "unsupported.yaml")
	wrongShape := filepath.Join(root, "wrong-shape.yaml")
	boolCommand := filepath.Join(root, "bool-command.yaml")

	write(t, invalidYAML, "copy: [\n")
	write(t, unsafeCopy, "copy:\n  - ../.env\n")
	write(t, unsafeSymlink, "symlink:\n  - /tmp/.env\n")
	write(t, unsupported, "commands:\n  - pnpm install\n")
	write(t, wrongShape, "copy: .env\n")
	write(t, boolCommand, "postSetup:\n  - false\n")

	loadErrorContains(t, invalidYAML, "invalid YAML")
	loadErrorContains(t, unsafeCopy, `cannot contain ".."`)
	loadErrorContains(t, unsafeSymlink, "relative file path")
	loadErrorContains(t, unsupported, "unsupported key")
	loadErrorContains(t, wrongShape, "must be an array")
	loadErrorContains(t, boolCommand, "non-empty string")
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
	_, err := LoadFile(filePath)
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
