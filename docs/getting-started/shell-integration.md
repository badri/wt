# Shell Integration

## Shell Completions

wt provides command completions for bash, zsh, and fish.

### Bash

Add to your `~/.bashrc`:

```bash
eval "$(wt completion bash)"
```

### Zsh

Add to your `~/.zshrc`:

```bash
eval "$(wt completion zsh)"
```

### Fish

Generate the completion file:

```bash
wt completion fish > ~/.config/fish/completions/wt.fish
```

## Tmux Keybindings

wt provides convenient tmux keybindings for session management.

### Setup

Add to your `~/.tmux.conf`:

```bash
wt keys >> ~/.tmux.conf
tmux source-file ~/.tmux.conf
```

### Available Keybindings

| Keybinding | Action |
|------------|--------|
| `C-b W` | Session picker popup (requires fzf) |
| `C-b N` | Create new session prompt |
| `C-b M` | Watch dashboard popup |
| `C-b H` | Jump to hub session |
| `C-b D` | Detach from hub |

### Session Picker

The session picker (`C-b W`) uses fzf to let you quickly switch between sessions:

```
┌─ Pick Session ─────────────────────────────────────────┐
│ > toast     myproject-abc   Working   Add auth flow   │
│   shadow    myproject-def   Idle      Fix login bug   │
│   obsidian  myproject-ghi   Working   Update tests    │
└────────────────────────────────────────────────────────┘
```

Type to filter, press Enter to switch.

## Interactive Picker

Without tmux keybindings, you can use the interactive picker:

```bash
wt pick
```

This works with fzf if available, or falls back to a numbered prompt.

## Shell History

wt sends commands to tmux sessions using `tmux send-keys`. To prevent these commands from appearing in your shell history, configure your shell to ignore commands that start with a space.

### Bash

Add to your `~/.bashrc`:

```bash
HISTCONTROL=ignorespace
```

Or to ignore both space-prefixed commands and duplicates:

```bash
HISTCONTROL=ignoreboth
```

### Zsh

Add to your `~/.zshrc`:

```zsh
setopt HIST_IGNORE_SPACE
```

With this configuration, commands sent by wt (which are prefixed with a space) won't appear in your shell history.
