package shellinit

import "fmt"

func Generate(shell string) (string, error) {
	switch shell {
	case "zsh":
		return legacyWrapperCleanupScript + zshCompletionScript, nil
	case "bash":
		return legacyWrapperCleanupScript + bashCompletionScript, nil
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}

const legacyWrapperCleanupScript = `case "$(typeset -f wktree 2>/dev/null)" in
  *WKTREE_CD_FILE*WKTREE_SETUP_FILE*) unset -f wktree ;;
esac

`

const zshCompletionScript = `
_wktree_completion() {
  local -a __wktree_values

  if (( CURRENT == 2 )); then
    __wktree_values=(doctor list cleanup new close remove switch init completion)
    _describe 'command' __wktree_values
    return
  fi

  if (( CURRENT >= 3 )); then
    __wktree_values=("${(@f)$(command wktree __complete "${words[2]}" "${words[CURRENT]}" 2>/dev/null)}")
    _describe 'value' __wktree_values
  fi
}

compdef _wktree_completion wktree
`

const bashCompletionScript = `
_wktree_completion() {
  local __wktree_cur
  local __wktree_command

  COMPREPLY=()
  __wktree_cur="${COMP_WORDS[COMP_CWORD]}"

  if [ "$COMP_CWORD" -eq 1 ]; then
    COMPREPLY=( $(compgen -W "doctor list cleanup new close remove switch init completion" -- "$__wktree_cur") )
    return 0
  fi

  if [ "$COMP_CWORD" -ge 2 ]; then
    __wktree_command="${COMP_WORDS[1]}"
    COMPREPLY=( $(command wktree __complete "$__wktree_command" "$__wktree_cur" 2>/dev/null) )
  fi
}

complete -F _wktree_completion wktree
`
