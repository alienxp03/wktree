# wktree

Create strict Git worktrees from the current `HEAD` or another ref, then open a tmux workspace for one repo or a related set of repos.

## Install

```bash
go install github.com/alienxp03/wktree/cmd/wktree@latest
```

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
wktree init
wktree doctor
wktree list
wktree new feature/example
wktree new --from main feature/example
wktree new --workspaces feature/example
wktree switch feature/example
wktree switch --workspaces feature/example
wktree remove feature/example
wktree remove --dry-run --workspaces feature/example
wktree remove --force --workspaces feature/example
```

`wktree new <branch>` creates a new branch and worktree for the first configured workspace. Use `--from <ref>` to create the branch from another local branch, remote branch, tag, or commit.

`wktree new --workspaces <branch>` creates the same branch across every configured workspace repo and opens them together in tmux.

`wktree switch <branch>` opens an existing branch worktree. If the branch exists but has no worktree yet, it creates one. If only `origin/<branch>` exists, it creates a local tracking branch and worktree.

`wktree remove <branch>` kills the matching tmux window or session target, removes the branch worktree, and deletes the local branch with Git's safe deletion rules. It does not remove remote branches. Use `--dry-run` to preview the tmux and Git actions. Use `--force` to remove a dirty worktree and force-delete the local branch. If `.wktree.env` shows that the branch was opened with multiple workspaces, single-workspace remove stops and asks for `--workspaces`.

`wktree list` shows every Git worktree for the current repository, including the primary checkout.

By default, worktrees are created under:

```text
~/workspace/worktrees
```

The target path format is:

```text
<worktree_home>/<owner_slug>/<repo_slug>/<branch_slug>
```

For GitHub remotes, `owner_slug` and `repo_slug` come from the remote URL. Without a supported remote, `owner_slug` comes from `git config user.name` and `repo_slug` comes from the repo directory name.

## Config

`wktree` reads only the project-local config:

```text
.wktree.yaml
```

Create a starter config in the current Git repository with:

```bash
wktree init
```

Single-repo example:

```yaml
worktree_dir: ~/worktree
tmux_mode: window
workspace_mode: single

defaults:
  files:
    copy:
      - .env
    symlink:
      - AGENTS.override.md

workspaces:
  - name: backend
    files:
      copy:
        - .env.backend
      symlink:
        - .tool-versions
    hooks:
      post_create:
        - gcloud auth application-default login
    panes:
      - command: nvim
        focus: true
      - commands:
          - pnpm install
          - pnpm run dev
        split: horizontal
      - command: codex
        split: vertical
```

Multi-repo example:

```yaml
worktree_dir: ~/worktree
workspace_mode: all

workspaces:
  - name: backend
    # repo omitted, so this workspace uses the current repo
    files:
      copy:
        - .env.backend
    hooks:
      post_create:
        - pnpm install
    randomize_ports:
      - file: .env.local
        vars:
          - PORT
          - APP_PORT
    panes:
      - command: nvim
        focus: true
      - commands:
          - pnpm install
          - pnpm run dev
        split: horizontal

  - name: frontend
    repo: ~/workspace/frontend
    files:
      copy:
        - .env.frontend
    hooks:
      post_create:
        - pnpm install
    panes:
      - command: nvim
        focus: true
      - command: pnpm run dev
        split: horizontal
```

`workspaces` is ordered. With `workspace_mode: single`, `wktree` uses the first item unless `--workspaces` is passed. With `workspace_mode: all`, `new`, `switch`, and `remove` use every workspace by default.

All-workspace runs always use a branch-scoped tmux session, regardless of `tmux_mode`. `tmux_mode` only controls single-workspace runs.

Each workspace becomes a tmux window. If `repo` is omitted, the workspace uses the current repo. Each `panes` item is one tmux pane. A pane can run either one `command` or multiple `commands`; multiple commands run in the same pane with `&&`.

`defaults.files` applies to every selected workspace first. Workspace-level `files` appends after those defaults. Hooks are workspace-specific and run before tmux opens.

`copy` creates an isolated file in the new worktree. `symlink` creates a link back to the source repository file, so later edits are shared by every linked worktree.

`randomize_ports` updates copied env files in the worktree with available localhost ports. Variables are explicit so unrelated numeric values are not changed:

```yaml
workspaces:
  - name: app
    files:
      copy:
        - .env
        - .env.local
    randomize_ports:
      - file: .env
        vars:
          - PORT
      - file: .env.local
        vars:
          - APP_PORT
          - API_PORT
```

Fresh worktrees get new port values. Switching back to an existing worktree preserves valid numeric values already present in the copied env files.

## Workspace Env

When a workspace is created or switched, `wktree` writes:

```text
.wktree.env
```

The file gives each selected workspace a way to find the others. It only contains one absolute path variable per selected workspace:

```sh
export WKTREE_BACKEND_DIR='/Users/stan/worktree/org/backend/feature-example'
export WKTREE_FRONTEND_DIR='/Users/stan/worktree/org/frontend/feature-example'
```

Variable names are derived from workspace names as `WKTREE_<WORKSPACE_NAME>_DIR`, uppercased with non-alphanumeric runs replaced by `_`. Names that would collide after sanitizing, such as `front-end` and `front end`, are rejected when the config loads.

Pane commands and `post_create` hooks source this env file before running. `wktree` does not edit `.gitignore`; `remove` deletes its generated env file before removing the worktree.

## Tmux

`wktree` always uses tmux.

`tmux_mode: window` is the default for single-workspace runs. It requires running inside tmux and creates one window in the current session. Window names use the configured workspace name:

```text
<workspace_name>
```

`tmux_mode: session` creates or opens one tmux session for the branch/workspace set, with one window per selected workspace. All-workspace runs use this session layout automatically:

```text
session: <owner_slug>-<repo_slug>/<branch_slug>
window:  <workspace_name>
```

## Shell Integration

Shell integration is completion-only:

```bash
eval "$(wktree completion zsh)"
```

```bash
eval "$(wktree completion bash)"
```

Completion includes commands, flags, existing local and `origin` branches for `switch`, and removable worktree branches for `remove`.

## Troubleshooting

- `tmux window mode requires running inside tmux`: start tmux first or set `tmux_mode: session`.
- `local branch already exists`: choose a branch name that does not exist locally.
- `origin branch already exists`: fetches or remote refs already include that branch.
- `branch does not exist locally or on origin`: use `wktree new <branch>` to create a new branch.
- `branch is not merged into current HEAD`: merge the branch or use `wktree remove --force <branch>` if you really want to delete it.
- `target worktree path already exists`: remove or rename the existing directory, or use `--home`.
- `tmux ... failed`: install tmux or choose names that produce non-conflicting tmux targets.
- `wktree doctor`: run this first when repository detection, config, or tmux looks wrong.
