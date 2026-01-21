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

## Claude Permissions

By default, wt launches Claude with `--dangerously-skip-permissions`. This flag:

- **Skips permission prompts** - Claude can read/write files, run commands, etc. without asking
- **Enables autonomous operation** - Workers can complete tasks without human intervention
- **Required for automation** - Features like `wt auto` depend on unattended execution

!!! warning "Security Consideration"
    This flag gives Claude full access to your system within the session. Only use wt in trusted environments with code you trust.

### Customizing the Editor Command

To use a different command or remove the flag:

```bash
wt config set editor_cmd "claude"  # Interactive mode with permission prompts
wt config set editor_cmd "claude --dangerously-skip-permissions --model sonnet"  # Custom flags
```

See [Configuration Reference](../reference/configuration.md) for all options.

## Claude Code Skill Setup

**This is the most important step.** Most wt usage happens through Claude conversations, not direct CLI commands.

### Install the wt Skill

Copy the skill to your Claude Code skills directory:

```bash
# Create skills directory if it doesn't exist
mkdir -p ~/.claude/skills

# Copy the wt skill
cp -r /path/to/wt/examples/claude-code-skill ~/.claude/skills/wt
```

Or if you installed via npm/go, download directly:

```bash
mkdir -p ~/.claude/skills
curl -sL https://raw.githubusercontent.com/badri/wt/main/examples/claude-code-skill/SKILL.md \
  -o ~/.claude/skills/wt.md
```

### Verify Skill Installation

In a Claude Code session, try:

```
/wt
```

Claude should recognize the wt skill and show session management capabilities.

### How You'll Use wt

In practice, you interact with wt through Claude conversations:

```
You: "What workers are running?"
Claude: [runs wt, shows active sessions]

You: "Spawn a worker for wt-abc"
Claude: [runs wt new wt-abc, reports result]

You: "Check on toast"
Claude: [runs wt toast to switch to that session]
```

The skill teaches Claude the wt commands and workflows, so you don't need to remember CLI syntax.

## Next Steps

- [Quick Start](quickstart.md) - Create your first session
- [Shell Integration](shell-integration.md) - Set up completions and keybindings
