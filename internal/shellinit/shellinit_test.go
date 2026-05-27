package shellinit

import (
	"os/exec"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	init, err := Generate("zsh")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"typeset -f wktree", "WKTREE_CD_FILE", "WKTREE_SETUP_FILE", "unset -f wktree", "compdef _wktree_completion wktree", "command wktree __complete", "doctor list new close remove switch init completion"} {
		if !strings.Contains(init, want) {
			t.Fatalf("init missing %q:\n%s", want, init)
		}
	}
	for _, removed := range []string{"wktree()", `command wktree "$@"`, `cd "$__wktree_target_dir"`} {
		if strings.Contains(init, removed) {
			t.Fatalf("init should not include %q:\n%s", removed, init)
		}
	}
	bash, err := Generate("bash")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bash, "complete -F _wktree_completion wktree") {
		t.Fatalf("bash init missing completion:\n%s", bash)
	}
	if _, err := Generate("fish"); err == nil {
		t.Fatal("expected unsupported shell error")
	}
}

func TestGenerateRemovesLegacyWrapper(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	init, err := Generate("bash")
	if err != nil {
		t.Fatal(err)
	}
	script := `wktree() {
  WKTREE_CD_FILE=x WKTREE_SETUP_FILE=y command wktree "$@"
}
` + init + `
if typeset -f wktree >/dev/null 2>&1; then
  exit 1
fi
`
	if output, err := exec.Command(bash, "-lc", script).CombinedOutput(); err != nil {
		t.Fatalf("legacy wrapper cleanup failed: %v\n%s", err, output)
	}
}
