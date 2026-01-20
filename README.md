# wt - Worktree Session Manager

Minimal agentic coding orchestrator built on:
- **Beads** for task tracking
- **Git worktrees** for isolation
- **Tmux** for session persistence
- **Claude** (or other agents) for execution

## Philosophy

**One bead = one session = one worktree.**

Each task (bead) gets its own isolated environment: a dedicated git worktree, a persistent tmux session, and an AI agent working on just that task. Sessions persist until you explicitly close them—no auto-compaction, no context loss, no handoff complexity.

The **hub-and-spoke model** keeps you in control:
- **Hub**: Your orchestration session where you groom beads, spawn workers, and monitor progress
- **Workers**: Isolated sessions where AI agents execute tasks autonomously

This separation means you can run multiple agents in parallel without them stepping on each other, while maintaining visibility into what each one is doing.

## Documentation

📚 **[Full Documentation](docs/index.md)** — Installation, concepts, command reference, and guides.

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

## Key Features

**Session Management**
- `wt new <bead>` — Spawn isolated worker sessions
- `wt watch` — Live TUI dashboard with status indicators
- `wt pick` — Interactive session picker (fzf integration)
- `wt signal` — Communicate session status (ready, blocked, error)

**Hub Orchestration**
- `wt hub` — Dedicated orchestration session
- `wt auto` — Autonomous batch processing of ready beads
- `wt ready` — See what work is available across projects

**Session History**
- `wt seance` — Talk to past sessions ("Why did you make this decision?")
- `wt events` — Full event history with filtering
- `wt handoff` — Hand off to fresh Claude instance with context

**Multi-Project Support**
- Register multiple projects with different configurations
- Per-project merge modes (direct, PR with auto-merge, PR with review)
- Port isolation for parallel test environments

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

## Claude Code Skill Setup

**Most wt usage happens through Claude conversations, not direct CLI.** Install the skill:

```bash
mkdir -p ~/.claude/skills
curl -sL https://raw.githubusercontent.com/badri/wt/main/examples/claude-code-skill/SKILL.md \
  -o ~/.claude/skills/wt.md
```

Then in Claude Code, use `/wt` to access wt commands through conversation.

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
