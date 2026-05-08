package shellinit

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	init, err := Generate("zsh")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"wktree()", "WKTREE_CD_FILE", "WKTREE_SETUP_FILE", `command wktree "$@"`, "compdef _wktree_completion wktree", "command wktree __complete"} {
		if !strings.Contains(init, want) {
			t.Fatalf("init missing %q:\n%s", want, init)
		}
	}
	if strings.Index(init, `cd "$__wktree_target_dir"`) > strings.Index(init, `command wktree __setup`) {
		t.Fatal("expected cd before deferred setup")
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
