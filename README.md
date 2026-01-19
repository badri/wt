# wt - Worktree Session Manager

Minimal agentic coding orchestrator built on:
- **Beads** for task tracking
- **Git worktrees** for isolation
- **Tmux** for session persistence
- **Claude** (or other agents) for execution

## Philosophy

One bead = one session = one worktree. Sessions persist until you explicitly close them. No auto-compaction, no handoff complexity.

## Status

**Work in progress.** See [SPEC.md](SPEC.md) for the full specification.

## Quick Overview

```bash
# In your hub (Claude session), groom beads
bd ready                    # See available work

# Spawn a worker session
wt new supabyoi-pks         # Creates worktree + tmux session + launches Claude
# → "Spawned session 'toast' for bead supabyoi-pks"

# List active sessions
wt
# ┌─ Active Sessions ─────────────────────────────────────────┐
# │ 🟢 toast    supabyoi-pks   Working   Auto-harden VM      │
# │ 🟡 shadow   supabyoi-g4a   Idle      Encrypt secrets     │
# └───────────────────────────────────────────────────────────┘

# Switch to a session (lands you in Claude conversation)
wt toast

# Complete work
wt done                     # Commits, pushes, creates PR
wt close toast              # Cleanup session + worktree
```

## Installation

**npm** (recommended for macOS - bypasses Gatekeeper):
```bash
npm install -g @worktree/wt
```

**Go**:
```bash
go install github.com/badri/wt/cmd/wt@latest
```

**From source**:
```bash
git clone https://github.com/badri/wt.git
cd wt
make install
```

**macOS Gatekeeper note**: If using `go install` and blocked by Gatekeeper:
```bash
xattr -d com.apple.quarantine $(which wt)
```

## Shell Completions

```bash
# Bash - add to ~/.bashrc
eval "$(wt completion bash)"

# Zsh - add to ~/.zshrc
eval "$(wt completion zsh)"

# Fish
wt completion fish > ~/.config/fish/completions/wt.fish
```

## Tmux Keybindings

```bash
# Add to ~/.tmux.conf
wt keys >> ~/.tmux.conf
tmux source-file ~/.tmux.conf
```

Then use `C-b s` to pick sessions with fzf.

## Related Projects

- [beads](https://github.com/steveyegge/beads) - Git-native issue tracking
- [gastown](https://github.com/steveyegge/gastown) - Multi-agent workspace manager (more complex)
- [vibekanban](https://github.com/BloopAI/vibe-kanban) - AI agent orchestration with visual UI

## License

MIT
