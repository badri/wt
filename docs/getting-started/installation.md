# Installation

## Prerequisites

Before installing wt, ensure you have:

- **Git** (2.17+) - for worktree support
- **Tmux** (3.0+) - for session management
- **Beads** - for task tracking ([install beads](https://github.com/steveyegge/beads))

## Installation Methods

### npm (Recommended for macOS)

The npm package is the easiest installation method and bypasses macOS Gatekeeper:

```bash
npm install -g @worktree/wt
```

### Go Install

If you have Go installed:

```bash
go install github.com/badri/wt/cmd/wt@latest
```

!!! warning "macOS Gatekeeper"
    If macOS blocks the binary, remove the quarantine attribute:
    ```bash
    xattr -d com.apple.quarantine $(which wt)
    ```

### From Source

Clone and build from source:

```bash
git clone https://github.com/badri/wt.git
cd wt
make install
```

## Verify Installation

Check that wt is installed correctly:

```bash
wt version
```

You should see version information displayed.

## Initial Setup

After installation, run the doctor command to check your setup:

```bash
wt doctor
```

This will verify that all dependencies are installed and configured correctly.

## Next Steps

- [Quick Start](quickstart.md) - Create your first session
- [Shell Integration](shell-integration.md) - Set up completions and keybindings
