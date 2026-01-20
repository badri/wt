# wt - Worktree Session Manager

**Minimal agentic coding orchestrator** built on:

- **[Beads](https://github.com/steveyegge/beads)** for task tracking
- **Git worktrees** for code isolation
- **Tmux** for session persistence
- **Claude** (or other agents) for execution

## Philosophy

**One bead = one session = one worktree.**

Sessions persist until you explicitly close them. No auto-compaction, no handoff complexity. Each worker session is isolated in its own git worktree, working on exactly one bead.

## Why wt?

When you're orchestrating multiple AI coding agents, you need:

1. **Isolation** - Each agent works in its own directory, no conflicts
2. **Persistence** - Sessions survive disconnects, pick up where you left off
3. **Visibility** - See what all your agents are doing at a glance
4. **Integration** - Native integration with your existing git workflow

wt gives you all of this with a simple CLI.

## Quick Example

```bash
# See what work is ready
wt ready

# Spawn a worker session
wt new myproject-abc
# → "Spawned session 'toast' for bead myproject-abc"

# List active sessions
wt
# ┌─ Active Sessions ────────────────────────────────────────┐
# │ 🟢 toast    myproject-abc   Working   Add auth flow     │
# │ 🟡 shadow   myproject-def   Idle      Fix login bug     │
# └──────────────────────────────────────────────────────────┘

# Switch to a session
wt toast

# Watch all sessions live
wt watch
```

## Getting Started

Ready to dive in?

- [Installation](getting-started/installation.md) - Get wt installed on your system
- [Quick Start](getting-started/quickstart.md) - Your first wt session in 5 minutes
- [Shell Integration](getting-started/shell-integration.md) - Completions and keybindings

## Core Concepts

- [Sessions](concepts/sessions.md) - How wt manages agent sessions
- [Worktrees](concepts/worktrees.md) - Git worktree isolation
- [Beads Integration](concepts/beads.md) - Task tracking with beads
- [Test Environments](concepts/test-environments.md) - Per-session test isolation
- [Merge Modes](concepts/merge-modes.md) - PR workflows

## Related Projects

- [beads](https://github.com/steveyegge/beads) - Git-native issue tracking
- [gastown](https://github.com/steveyegge/gastown) - Multi-agent workspace manager
- [vibekanban](https://github.com/BloopAI/vibe-kanban) - AI agent orchestration with visual UI
