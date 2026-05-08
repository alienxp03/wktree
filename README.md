# wktree

Create strict Git worktrees from the current `HEAD`, switch to existing branch worktrees, optionally cd into them through shell integration or open them in tmux.

## Install

```bash
go install github.com/alienxp03/wktree/cmd/wktree@latest
```

Release binaries and Homebrew install instructions will be available from GitHub Releases once the first release is published.

For local development from this repository:

```bash
make build
./dist/wktree --help
```

Install the local build to your user bin path:

```bash
make install
```

This installs to `~/.local/bin/wktree` by default. Override the target directory with:

```bash
make install INSTALL_DIR="$HOME/bin"
```

## Usage

```bash
wktree list
wktree new feature/example
wktree switch feature/example
wktree remove feature/example
wktree remove --force feature/example
wktree new --tmux feature/example
wktree switch --tmux feature/example
wktree new --home /tmp/worktrees feature/example
```

`wktree new <branch>` creates a new branch from the current `HEAD` and fails if that branch already exists locally or on `origin`.

`wktree switch <branch>` opens an existing branch worktree. If the branch exists but has no worktree yet, it creates one. If only `origin/<branch>` exists, it creates a local tracking branch and worktree. If the branch already has a worktree, it reuses that path.

`wktree remove <branch>` kills the matching tmux session if it exists, removes the branch worktree, and then deletes the local branch with Git's safe branch deletion rules. It does not remove remote branches. Use `--force` to remove a dirty worktree and force-delete the local branch.

`wktree list` shows every Git worktree for the current repository, including the primary checkout.

By default, worktrees are created under:

```text
~/workspace/worktrees
```

The target path format is:

```text
<worktree_home>/<org>_<repo>_<branch_slug>
```

## Shell Integration

A command line program cannot cd the parent shell by itself. Add one of these to your shell profile:

```bash
eval "$(wktree init zsh)"
```

```bash
eval "$(wktree init bash)"
```

Then `wktree new feature/example` or `wktree switch feature/example` changes your shell into the resolved worktree. The same shell integration adds tab completion for commands and branch names. Use `--no-cd` to keep the plain executable behavior.

Completion includes existing local and `origin` branches for `wktree switch`, and removable worktree branches for `wktree remove`.

## Setup Config

Global config:

```text
$XDG_CONFIG_HOME/wktree/config.yaml
```

Fallback global config:

```text
~/.config/wktree/config.yaml
```

Project config:

```text
.wktree.yaml
```

Example:

```yaml
copy:
  - .env
symlink:
  - .mcp.json
postSetup:
  - gcloud auth application-default login
```

Project config is merged after global config:

```yaml
copy:
  - .env.local
symlink:
  - .tool-versions
postSetup:
  - pnpm install
```

`copy` creates an isolated file in the new worktree. `symlink` creates a link back to the source repository file, so later edits are shared by every linked worktree.
All setup keys are optional; omit `copy`, `symlink`, or `postSetup` when you do not need them.

Use `--no-setup` to skip configured file copies, symlinks, and post-setup commands.

## Tmux

```bash
wktree new --tmux feature/example
wktree switch --tmux feature/example
```

With `--tmux`, wktree creates a new session rooted at the worktree. The session is named from the last two worktree path components, and the first window is named from the current worktree directory.

For example:

```text
worktree path: /Users/azuan.zairein/workspace/worktrees/testing__feature-update-readme-1
session name:  worktrees/testing__feature-update-readme-1
window name:   testing__feature-update-readme-1
```

Inside tmux, wktree switches the current client to the new session. Outside tmux, it attaches to the new session.

## Troubleshooting

- `local branch already exists`: choose a branch name that does not exist locally.
- `origin branch already exists`: fetches or remote refs already include that branch.
- `branch does not exist locally or on origin`: use `wktree new <branch>` to create a new branch.
- `branch is not merged into current HEAD`: merge the branch or use `wktree remove --force <branch>` if you really want to delete it.
- `target worktree path already exists`: remove or rename the existing directory, or use `--home`.
- `tmux ... failed`: install tmux or choose a branch name that produces a non-conflicting tmux target.
