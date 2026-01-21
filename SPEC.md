# wt - Worktree Session Manager Specification

> **Note**: The full technical specification has been moved to the documentation.
> See [docs/reference/spec.md](docs/reference/spec.md) for complete details.

For user-focused documentation, see:
- [Hub Workflow Guide](docs/guides/hub-workflow.md) - Conversational orchestration
- [Sample Workflows](docs/guides/sample-workflows.md) - Practical scenarios

---

## Quick Overview

`wt` is a minimal agentic coding orchestrator built on:
- **Beads** for task tracking
- **Git worktrees** for isolation
- **Tmux** for session persistence
- **Claude** (or other agents) for execution

**One bead = one session = one worktree.**

### Hub-and-Spoke Model

```
┌─────────────────────────────────────────────────────────────┐
│                          Hub                                 │
│  (You + Claude orchestrating work)                          │
└─────────────────────────────────────────────────────────────┘
         │              │              │
         ▼              ▼              ▼
    ┌─────────┐   ┌─────────┐   ┌─────────┐
    │ Worker  │   │ Worker  │   │ Worker  │
    │ toast   │   │ shadow  │   │ obsidian│
    └─────────┘   └─────────┘   └─────────┘
```

- **Hub**: Your control center for grooming beads, spawning workers, and monitoring progress.
- **Workers**: Isolated sessions where AI agents execute tasks autonomously.

### Design Principles

1. **Explicit over automatic**: You control the lifecycle.
2. **Isolation over sharing**: Each worker has its own worktree and branch.
3. **Visibility over opacity**: `wt watch` shows all sessions at a glance.
4. **Simplicity over features**: Core commands are `new`, `switch`, `done`, `close`.

---

For the full specification including configuration formats, command reference, and implementation details, see [docs/reference/spec.md](docs/reference/spec.md).
