# wt - Worktree Session Manager

**Minimal agentic coding orchestrator** built on:

- **[Beads](https://github.com/steveyegge/beads)** for task tracking
- **Git worktrees** for code isolation
- **Tmux** for session persistence
- **Claude** (or other agents) for execution

## Philosophy

**One bead = one session = one worktree.**

Each task (bead) gets its own isolated environment: a dedicated git worktree, a persistent tmux session, and an AI agent working on just that task. Sessions persist until you explicitly close them—no auto-compaction, no context loss, no handoff complexity.

### Hub-and-Spoke Model

The architecture separates orchestration from execution:

- **Hub**: Your control center for grooming beads, spawning workers, and monitoring progress
- **Workers**: Isolated sessions where AI agents execute tasks autonomously

This separation means you can run multiple agents in parallel without conflicts, while maintaining visibility into what each one is doing.

### Design Principles

1. **Explicit over automatic** — Sessions don't auto-close. You control the lifecycle.
2. **Isolation over sharing** — Each worker has its own worktree, branch, and test environment.
3. **Visibility over opacity** — `wt watch` shows all sessions at a glance.
4. **Simplicity over features** — Core commands are just `new`, `switch`, `done`, `close`.

## Why wt?

When you're orchestrating multiple AI coding agents, you need:

1. **Isolation** — Each agent works in its own directory, no conflicts
2. **Persistence** — Sessions survive disconnects, pick up where you left off
3. **Visibility** — See what all your agents are doing at a glance
4. **Integration** — Native integration with your existing git workflow

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

## Guides

Practical guides for using wt effectively:

- [Hub Workflow](guides/hub-workflow.md) - The hub/worker pattern and conversational orchestration
- [Sample Workflows](guides/sample-workflows.md) - Real-world scenarios: PR review, YOLO mode, parallel workers, and more
- [Auto Mode](guides/auto-mode.md) - Hands-off batch processing
- [Seance](guides/seance.md) - Querying past sessions

## Core Concepts

- [Overview](concepts/overview.md) - How wt works
- [Sessions](concepts/sessions.md) - How wt manages agent sessions
- [Worktrees](concepts/worktrees.md) - Git worktree isolation
- [Beads Integration](concepts/beads.md) - Task tracking with beads
- [Test Environments](concepts/test-environments.md) - Per-session test isolation
- [Merge Modes](concepts/merge-modes.md) - PR workflows

## Reference

- [Technical Specification](reference/spec.md) - Architecture, configuration, and internals
- [Configuration](reference/configuration.md) - All configuration options
- [Environment](reference/environment.md) - Environment variables
- [Troubleshooting](reference/troubleshooting.md) - Common issues and solutions

## Related Projects

- [beads](https://github.com/steveyegge/beads) - Git-native issue tracking
- [gastown](https://github.com/steveyegge/gastown) - Multi-agent workspace manager
- [vibekanban](https://github.com/BloopAI/vibe-kanban) - AI agent orchestration with visual UI
