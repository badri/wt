# Utility Commands

Diagnostic, integration, and helper commands.

## Diagnostics

### `wt doctor`

Diagnose wt setup issues.

```bash
wt doctor
```

Checks:

- Git version and worktree support
- Tmux version
- Beads installation
- Configuration validity
- Project registrations

Output:
```
✓ Git 2.39.0 (worktree support)
✓ Tmux 3.3a
✓ Beads found at ~/.local/bin/bd
✓ Config file valid
✓ 2 projects registered
  - myproject: ~/code/myproject
  - other: ~/code/other
```

### `wt events`

Show wt event log.

```bash
wt events
```

**Options:**

| Flag | Description |
|------|-------------|
| `--follow`, `-f` | Tail events in real-time |
| `--since <duration>` | Show events since duration (e.g., `1h`, `30m`) |
| `-n <count>` | Show last N events |

Examples:

```bash
wt events -n 20          # Last 20 events
wt events --since 1h     # Last hour
wt events --follow       # Live tail
```

Event log location: `~/.config/wt/events.jsonl`

---

## Shell Integration

### `wt completion <shell>`

Generate shell completion script.

```bash
wt completion bash
wt completion zsh
wt completion fish
```

**Setup:**

=== "Bash"
    ```bash
    # Add to ~/.bashrc
    eval "$(wt completion bash)"
    ```

=== "Zsh"
    ```bash
    # Add to ~/.zshrc
    eval "$(wt completion zsh)"
    ```

=== "Fish"
    ```bash
    wt completion fish > ~/.config/fish/completions/wt.fish
    ```

### `wt keys`

Output tmux keybinding configuration.

```bash
wt keys
```

**Setup:**

```bash
wt keys >> ~/.tmux.conf
tmux source-file ~/.tmux.conf
```

**Keybindings installed:**

| Key | Action |
|-----|--------|
| `C-b W` | Session picker popup |
| `C-b N` | Create new session prompt |
| `C-b M` | Watch dashboard popup |
| `C-b H` | Jump to hub session |
| `C-b D` | Detach from hub |

---

## Session Context

### `wt prime`

Inject startup context for new Claude session.

```bash
wt prime
wt prime --quiet
```

Used internally when spawning sessions. Provides Claude with:

- Session information
- Bead details
- Project context
- Available commands

### `wt pick`

Interactive session picker.

```bash
wt pick
```

- Uses fzf if available
- Falls back to numbered prompt

---

## Information

### `wt version`

Show version information.

```bash
wt version
```

Output:
```
wt version 0.2.0
commit: abc1234
built: 2026-01-19
```

### `wt help`

Show main help text.

```bash
wt help
wt --help
wt -h
```

Get help for specific commands:

```bash
wt new --help
wt config --help
```

---

## Advanced

### `wt handoff`

Hand off hub to fresh Claude instance.

```bash
wt handoff
wt handoff -m "Focus on bug fixes"
wt handoff -c           # Auto-collect state
wt handoff --dry-run    # Preview collection
```

See [Hub Commands](hub.md#handoff) for details.

### Environment Variables

wt respects these environment variables:

| Variable | Description |
|----------|-------------|
| `EDITOR` | Editor for `wt config edit` |
| `WT_CONFIG_DIR` | Override config directory |
| `WT_DEBUG` | Enable debug logging |

---

## Tmux Integration

### Popup Commands

From within tmux, these commands open popups:

```bash
# Session picker
tmux display-popup -E "wt pick"

# Watch dashboard
tmux display-popup -E -w 80% -h 80% "wt watch"

# New session prompt
tmux command-prompt -p "bead:" "run-shell 'wt new %%'"
```

### Status Line

Show current session in tmux status line:

```bash
# In ~/.tmux.conf
set -g status-right "#(wt status --short 2>/dev/null || echo 'no session')"
```
