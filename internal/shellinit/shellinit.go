package shellinit

import "fmt"

func Generate(shell string) (string, error) {
	switch shell {
	case "zsh":
		return wrapperScript + zshCompletionScript, nil
	case "bash":
		return wrapperScript + bashCompletionScript, nil
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}

const wrapperScript = `wktree() {
  local __wktree_status
  local __wktree_cd_file
  local __wktree_setup_file

  __wktree_cd_file="$(mktemp -t wktree-cd.XXXXXX)" || return 1
  __wktree_setup_file="$(mktemp -t wktree-setup.XXXXXX)" || {
    __wktree_status=$?
    rm -f "$__wktree_cd_file"
    return "$__wktree_status"
  }

  WKTREE_CD_FILE="$__wktree_cd_file" WKTREE_SETUP_FILE="$__wktree_setup_file" command wktree "$@"
  __wktree_status=$?

  if [ "$__wktree_status" -eq 0 ] && [ -s "$__wktree_cd_file" ]; then
    local __wktree_target_dir
    __wktree_target_dir="$(cat "$__wktree_cd_file")"
    if [ -n "$__wktree_target_dir" ]; then
      cd "$__wktree_target_dir"
      __wktree_status=$?
      if [ "$__wktree_status" -eq 0 ] && [ -s "$__wktree_setup_file" ]; then
        command wktree __setup "$__wktree_setup_file"
        __wktree_status=$?
      fi
    fi
  fi

  rm -f "$__wktree_cd_file" "$__wktree_setup_file"
  return "$__wktree_status"
}
`

const zshCompletionScript = `
_wktree_completion() {
  local -a __wktree_values

  if (( CURRENT == 2 )); then
    __wktree_values=(list new remove switch init)
    _describe 'command' __wktree_values
    return
  fi

  if (( CURRENT == 3 )); then
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
    COMPREPLY=( $(compgen -W "list new remove switch init" -- "$__wktree_cur") )
    return 0
  fi

  if [ "$COMP_CWORD" -eq 2 ]; then
    __wktree_command="${COMP_WORDS[1]}"
    COMPREPLY=( $(command wktree __complete "$__wktree_command" "$__wktree_cur" 2>/dev/null) )
  fi
}

complete -F _wktree_completion wktree
`
