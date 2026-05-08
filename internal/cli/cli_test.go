package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpAndVersion(t *testing.T) {
	stdout := &bytes.Buffer{}
	status := Run([]string{"--help"}, Options{Stdout: stdout, Stderr: &bytes.Buffer{}})
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stdout.String(), "wktree - create strict Git worktrees") {
		t.Fatalf("help = %q", stdout.String())
	}

	stdout.Reset()
	status = Run([]string{"--version"}, Options{Stdout: stdout, Stderr: &bytes.Buffer{}})
	if status != 0 || strings.TrimSpace(stdout.String()) != Version {
		t.Fatalf("version status=%d stdout=%q", status, stdout.String())
	}
}

func TestInit(t *testing.T) {
	stdout := &bytes.Buffer{}
	status := Run([]string{"init", "bash"}, Options{Stdout: stdout, Stderr: &bytes.Buffer{}})
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stdout.String(), "wktree()") {
		t.Fatalf("init = %q", stdout.String())
	}
}

func TestInvalidNewUsage(t *testing.T) {
	stderr := &bytes.Buffer{}
	status := Run([]string{"new"}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "usage: wktree new") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestInvalidListUsage(t *testing.T) {
	stderr := &bytes.Buffer{}
	status := Run([]string{"list", "extra"}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "usage: wktree list") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestInvalidRemoveUsage(t *testing.T) {
	stderr := &bytes.Buffer{}
	status := Run([]string{"remove"}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "usage: wktree remove") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestInvalidSwitchUsage(t *testing.T) {
	stderr := &bytes.Buffer{}
	status := Run([]string{"switch"}, Options{Stdout: &bytes.Buffer{}, Stderr: stderr})
	if status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "usage: wktree switch") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
