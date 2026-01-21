# Configuration Commands

Configuration is typically done once during setup. You can ask Claude to help:

```
You: "Register my project at ~/code/myapp"
Claude: [runs wt project add myapp ~/code/myapp] Done. How should completed work be merged?

You: "Configure it for PR review"
Claude: [updates config] Set merge_mode to pr-review.
```

This page documents the configuration commands and options for reference.

## Config Management

### `wt config show`

Display current configuration.

```bash
wt config show
```

Output:
```json
{
  "worktree_root": "~/worktrees",
  "editor_cmd": "claude --dangerously-skip-permissions",
  "default_merge_mode": "pr-review"
}
```

### `wt config init`

Initialize configuration file with defaults.

```bash
wt config init
```

Creates `~/.config/wt/config.json` if it doesn't exist.

### `wt config set <key> <value>`

Set a configuration option.

```bash
wt config set worktree_root ~/my-worktrees
wt config set default_merge_mode direct
wt config set editor_cmd "code --wait"
```

### `wt config edit`

Open configuration in your editor.

```bash
wt config edit
```

Uses `$EDITOR` environment variable.

---

## Configuration Options

### Global Options

| Key | Description | Default |
|-----|-------------|---------|
| `worktree_root` | Directory for worktrees | `~/worktrees` |
| `editor_cmd` | Command to launch Claude/editor | `claude --dangerously-skip-permissions` |
| `default_merge_mode` | Default merge strategy | `pr-review` |

### Project Options

Configure per-project in `~/.config/wt/projects/<name>.json`:

```json
{
  "name": "myproject",
  "repo": "~/code/myproject",
  "default_branch": "main",
  "beads_prefix": "myproject",

  "merge_mode": "pr-review",
  "require_ci": true,
  "auto_merge_on_green": false,

  "test_env": {
    "setup": "docker compose up -d",
    "teardown": "docker compose down",
    "port_env": "PORT_OFFSET",
    "health_check": "curl -f http://localhost:${PORT_OFFSET}3000/health"
  },

  "hooks": {
    "on_create": ["npm install"],
    "on_close": ["docker compose down"]
  }
}
```

---

## Configuration Files

### Directory Structure

```
~/.config/wt/
├── config.json         # Global configuration
├── sessions.json       # Active session state
├── namepool.txt        # Available session names
├── events.jsonl        # Event log
└── projects/
    ├── myproject.json  # Project-specific config
    └── other.json
```

### config.json

Global settings:

```json
{
  "worktree_root": "~/worktrees",
  "editor_cmd": "claude --dangerously-skip-permissions",
  "default_merge_mode": "pr-review"
}
```

### sessions.json

Active session state (managed by wt):

```json
{
  "toast": {
    "bead": "myproject-abc123",
    "project": "myproject",
    "worktree": "/Users/you/worktrees/toast",
    "branch": "myproject-abc123",
    "port_offset": 1,
    "beads_dir": "/Users/you/myproject/.beads",
    "created_at": "2026-01-19T08:30:00Z",
    "last_activity": "2026-01-19T10:45:00Z",
    "status": "working",
    "status_message": ""
  }
}
```

### namepool.txt

Session names, one per line:

```
toast
shadow
obsidian
quartz
jasper
ember
frost
coral
sage
dusk
```

Names are themed. Available themes:

- kung-fu-panda
- toy-story
- ghibli
- star-wars
- dune
- matrix

---

## Project Configuration Reference

### Basic Settings

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Project identifier |
| `repo` | string | Path to git repository |
| `default_branch` | string | Branch to merge into (default: `main`) |
| `beads_prefix` | string | Prefix for bead IDs |

### Merge Settings

| Field | Type | Description |
|-------|------|-------------|
| `merge_mode` | string | `direct`, `pr-auto`, or `pr-review` |
| `require_ci` | boolean | Wait for CI before merge |
| `auto_merge_on_green` | boolean | Auto-merge when CI passes |

### Test Environment

| Field | Type | Description |
|-------|------|-------------|
| `test_env.setup` | string | Command to start services |
| `test_env.teardown` | string | Command to stop services |
| `test_env.port_env` | string | Env var name for port offset |
| `test_env.health_check` | string | Command to verify readiness |

### Hooks

| Field | Type | Description |
|-------|------|-------------|
| `hooks.on_create` | string[] | Commands run when session created |
| `hooks.on_close` | string[] | Commands run when session closed |
