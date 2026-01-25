# wt - Claude Code Orchestrator

Claude Code orchestrator for bead-driven development.

## Installation

```bash
npm install -g @worktree/wt
```

## What is wt?

`wt` manages git worktree sessions for working on beads (issues) with Claude. Each session gets:

- Isolated git worktree with dedicated branch
- Tmux session with your editor
- Automatic bead tracking via BEADS_DIR

## Quick Start

```bash
# List ready beads
wt ready

# Start a new session
wt new <bead-id>

# List active sessions
wt list

# Switch to a session
wt <session-name>

# Complete and merge
wt done
```

## Requirements

- Git
- Tmux
- Beads (`bd` command for issue tracking)

## Alternative Installation

If npm installation fails, you can install via Go:

```bash
go install github.com/badri/wt/cmd/wt@latest
```

## More Information

- [GitHub Repository](https://github.com/badri/wt)
- [Issue Tracker](https://github.com/badri/wt/issues)
