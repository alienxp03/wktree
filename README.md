# wktree

`git worktree` + `tmux` for one or multiple repos.

## Motivation

- I work in tmux + neovim. Terminal is my IDE. I like to use `git worktree` to isolate work across features and PRs. 
- More than often, I need to work across multiple repos for one feature (e.g. `backend` + `frontend`). 
- Inspired by [workmux](https://github.com/raine/workmux), but I needed multi-repo support.

## Disclaimer
- *Looks like vibe coded?* : **Yes. Good vibes only.**

- *Do you even use this?* : **Yes, daily.**

## Install

### Homebrew (recommended)

```bash
brew tap alienxp03/tap
brew install wktree
```

### Go

```bash
go install github.com/alienxp03/wktree/cmd/wktree@latest
```

## Setup

```bash
wktree init
```

Creates `.wktree.yaml` in the current directory. `wktree` searches from cwd up to the Git root and uses the nearest config.

### Single-repo

```yaml
worktree_dir: ~/worktree
tmux:
  mode: window
  session_name: "${repo}/${branch}"
workspace_mode: single

workspaces:
  - name: backend
    files:
      copy:
        - .env.backend
      symlink:
        - .tool-versions
    hooks:
      post_create:
        - direnv allow
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

### Multi-repo

```yaml
worktree_dir: ~/worktree
workspace_mode: all

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
    hooks:
      post_create:
        - direnv allow
    randomize_ports:
      - file: .env.local
        vars:
          - PORT
          - APP_PORT
    set_env:
      - file: .env.local
        vars:
          API_URL: "http://localhost:${backend:.env.local:PORT}/api"
    open:
      - "http://localhost:${backend:.env.local:PORT}/api"
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

## Config reference

### Top-level

| Key | Description |
|-----|-------------|
| `worktree_dir` | Where worktrees are stored. Default `~/workspace/worktrees`. |
| `tmux.mode` | `window` (default) creates a tmux window in the current session. `session` creates a new session. |
| `tmux.session_name` | Tmux session name template. Supports `${repo}`, `${branch}`, `${dir}`, and `${dir:N}`. Default `${repo}/${branch}`. |
| `workspace_mode` | `single` uses the first workspace unless `--workspaces` is passed. `all` uses every workspace by default. |
| `defaults` | Shared config applied to every workspace before workspace-level config. |
| `workspaces` | Ordered list of workspaces. |

`${dir}` is the directory containing `.wktree.yaml`; `${dir:1}` is its parent, `${dir:2}` is the next parent, and so on.

### Workspace

| Key | Description |
|-----|-------------|
| `name` | Workspace identifier. Used for tmux window names and env variable names. |
| `repo` | Path to the Git repo. Omit to use the directory containing `.wktree.yaml`. |
| `files.copy` | Copy files into the new worktree (isolated per worktree). |
| `files.symlink` | Symlink files from the source repo (shared across worktrees). |
| `hooks.post_create` | Commands to run after worktree creation, before tmux opens. |
| `randomize_ports` | Replaces named env vars with available localhost ports. |
| `set_env` | Sets env variables with template references like `${workspace:file:VAR}`. |
| `open` | Opens URLs after setup completes. Uses the same template references as `set_env`. |
| `panes` | Tmux panes. Each pane has `command` or `commands`, optional `split` (`horizontal`/`vertical`), and optional `focus`. |

### Workspace env

When a workspace is created or switched, `wktree` writes `.wktree.env` with one variable per workspace:

```sh
export WKTREE_BACKEND_DIR='/Users/stan/worktree/org/backend/feature-example'
export WKTREE_FRONTEND_DIR='/Users/stan/worktree/org/frontend/feature-example'
```

Pane commands and `post_create` hooks source this file before running. PR worktrees also include `WKTREE_PR_NUMBER`, `WKTREE_PR_URL`, `WKTREE_PR_HEAD_REF`, and `WKTREE_PR_HEAD_SHA`.

## Usage

```
wktree init                       Create starter config
wktree doctor                     Check repo, config, and tmux setup

wktree list                       List worktrees
wktree list --pr                  List worktrees with PR info
wktree cleanup                    Remove merged worktree branches after confirmation
wktree cleanup --dry-run          Preview merged worktree branch cleanup

wktree new <branch>               Create branch + worktree + tmux
wktree new --from <ref> <branch>  Create from a specific ref
wktree new --workspaces <branch>  Create across all workspaces

wktree switch <branch>            Open existing worktree in tmux
wktree switch --pr <number|url>   Open a GitHub PR locally

wktree close <branch>             Close tmux (keeps worktree)
wktree remove <branch>            Close tmux + remove worktree + delete branch
```

Common flags: `--dry-run` to preview, `--force` to override safety checks, `--workspaces` to target all workspaces.
Use `wktree cleanup --yes` to skip the confirmation prompt after reviewing the cleanup behavior.

Worktree paths follow `<worktree_dir>/<owner>/<repo>/<branch>`. For GitHub remotes, `owner` and `repo` come from the remote URL.

## Shell integration

Completion-only:

```bash
eval "$(wktree completion zsh)"   # or bash
```

## Development

```bash
make build                        # build to dist/
make install                      # install to ~/.local/bin/wktree
make lint test                    # lint and test
```

## Troubleshooting

- **`tmux window mode requires running inside tmux`** — start tmux first or use `tmux.mode: session`.
- **`gh is required for --pr`** — install and authenticate the [GitHub CLI](https://cli.github.com/).
- **`branch is not merged`** — merge first, check whether the local branch has commits beyond the merged PR head, or use `--force`.
- **`wktree doctor`** — run this first when something looks wrong.
