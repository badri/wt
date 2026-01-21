# wt - Worktree Session Manager

**Minimal agentic coding orchestrator** where you talk to Claude to manage multiple parallel AI workers.

## How It Works

**You have a conversation with Claude. Claude runs the commands.**

```
You: "What work is ready?"
Claude: [runs wt ready] Three beads available: auth feature, login bug, error messages.

You: "Spawn a worker for the auth feature."
Claude: [runs wt new myproject-abc] Session 'toast' is working on it.

You: "How's it going?"
Claude: [runs wt] Toast is actively working, 3 commits so far.

You: "Toast says it's done. Close it."
Claude: [runs wt close toast] PR created, session cleaned up.
```

You describe what you want. Claude handles the wt commands, git operations, and worker management.

## The Hub/Worker Model

The architecture separates orchestration from execution:

- **Hub**: A conversation with Claude where you plan work, spawn workers, and monitor progress
- **Workers**: Isolated sessions where AI agents execute tasks autonomously

```
┌─────────────────────────────────────────────────────────────┐
│  HUB (You + Claude)                                         │
│                                                             │
│  You: "What's ready?"                                       │
│  You: "Spawn workers for the bug fixes"                     │
│  You: "How's toast doing?"                                  │
│  You: "Close the finished ones"                             │
│                                                             │
└─────────────────────┬───────────────────────────────────────┘
                      │ spawns/monitors
                      ▼
┌─────────────────────────────────────────────────────────────┐
│  WORKERS (Claude instances)                                 │
│                                                             │
│  toast: Working on auth feature in ~/worktrees/toast        │
│  shadow: Working on login bug in ~/worktrees/shadow         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**One bead = one session = one worktree.** Each task gets isolated: its own git worktree, tmux session, and AI agent.

## Built On

- **[Beads](https://github.com/steveyegge/beads)** for task tracking
- **Git worktrees** for code isolation
- **Tmux** for session persistence
- **Claude** (or other agents) for execution

## Design Principles

1. **Conversational over CLI** — You talk to Claude, not memorize commands.
2. **Isolation over sharing** — Each worker has its own worktree, branch, and test environment.
3. **Explicit over automatic** — Sessions don't auto-close. You control the lifecycle.
4. **Visibility over opacity** — Check on any worker anytime through the hub.

## Getting Started

- [Installation](getting-started/installation.md) - Get wt installed on your system
- [Quick Start](getting-started/quickstart.md) - Your first hub session in 5 minutes
- [Shell Integration](getting-started/shell-integration.md) - Completions and keybindings

## Guides

Learn how to work through your hub:

- [Hub Workflow](guides/hub-workflow.md) - The conversation-driven approach to orchestration
- [Sample Workflows](guides/sample-workflows.md) - Real-world scenarios with conversation examples
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

Technical reference for power users who want to understand what Claude runs behind the scenes:

- [Commands](commands/) - CLI reference (what Claude runs for you)
- [Configuration](reference/configuration.md) - All configuration options
- [Environment](reference/environment.md) - Environment variables
- [Technical Specification](reference/spec.md) - Architecture and internals
- [Troubleshooting](reference/troubleshooting.md) - Common issues and solutions

## Related Projects

- [beads](https://github.com/steveyegge/beads) - Git-native issue tracking
- [gastown](https://github.com/steveyegge/gastown) - Multi-agent workspace manager
- [vibekanban](https://github.com/BloopAI/vibe-kanban) - AI agent orchestration with visual UI
