# Configuration Reference

Complete reference for wt configuration options.

## Configuration Files

### Location

All configuration is stored in `~/.config/wt/`:

```
~/.config/wt/
├── config.json         # Global configuration
├── sessions.json       # Active session state (managed by wt)
├── namepool.txt        # Available session names
├── events.jsonl        # Event log
└── projects/
    ├── myproject.json  # Project configuration
    └── other.json
```

Override the config directory with `WT_CONFIG_DIR` environment variable.

---

## Global Configuration

**File**: `~/.config/wt/config.json`

```json
{
  "worktree_root": "~/worktrees",
  "editor_cmd": "claude --dangerously-skip-permissions",
  "default_merge_mode": "pr-review"
}
```

### Options

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `worktree_root` | string | `~/worktrees` | Directory where worktrees are created |
| `editor_cmd` | string | `claude --dangerously-skip-permissions` | Command to launch the coding agent |
| `default_merge_mode` | string | `pr-review` | Default merge strategy for all projects |

### Merge Modes

| Mode | Description |
|------|-------------|
| `direct` | Push directly to default branch |
| `pr-auto` | Create PR, auto-merge when CI passes |
| `pr-review` | Create PR, wait for human review |

---

## Project Configuration

**File**: `~/.config/wt/projects/<project>.json`

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
  },

  "namepool_theme": "star-wars"
}
```

### Basic Settings

| Key | Type | Required | Description |
|-----|------|----------|-------------|
| `name` | string | Yes | Project identifier |
| `repo` | string | Yes | Path to git repository |
| `default_branch` | string | No | Branch to merge into (default: `main`) |
| `beads_prefix` | string | No | Prefix for bead IDs |

### Merge Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `merge_mode` | string | (global) | `direct`, `pr-auto`, or `pr-review` |
| `require_ci` | boolean | `true` | Wait for CI before allowing merge |
| `auto_merge_on_green` | boolean | `false` | Auto-merge PRs when CI passes |

### Test Environment

Configure per-session test isolation:

| Key | Type | Description |
|-----|------|-------------|
| `test_env.setup` | string | Command to start test services |
| `test_env.teardown` | string | Command to stop test services |
| `test_env.port_env` | string | Environment variable for port offset |
| `test_env.health_check` | string | Command to verify services ready |

### Hooks

Commands run at session lifecycle points:

| Key | Type | Description |
|-----|------|-------------|
| `hooks.on_create` | string[] | Commands run after session created |
| `hooks.on_close` | string[] | Commands run before session closed |

### Namepool

| Key | Type | Description |
|-----|------|-------------|
| `namepool_theme` | string | Theme for session names |

Available themes:

- `kung-fu-panda`
- `toy-story`
- `ghibli`
- `star-wars`
- `dune`
- `matrix`

---

## Session State

**File**: `~/.config/wt/sessions.json` (managed by wt)

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

### Session Fields

| Field | Type | Description |
|-------|------|-------------|
| `bead` | string | Bead ID this session is working on |
| `project` | string | Project name |
| `worktree` | string | Path to worktree directory |
| `branch` | string | Git branch name |
| `port_offset` | number | Port offset for test isolation |
| `beads_dir` | string | Path to main repo's beads directory |
| `created_at` | string | ISO timestamp of creation |
| `last_activity` | string | ISO timestamp of last activity |
| `status` | string | Current status |
| `status_message` | string | Optional status message |

### Status Values

| Status | Description |
|--------|-------------|
| `working` | Actively making changes |
| `idle` | Waiting at prompt |
| `ready` | Work complete |
| `blocked` | Cannot proceed |
| `error` | Something went wrong |

---

## Namepool

**File**: `~/.config/wt/namepool.txt`

One name per line:

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

Names are assigned to sessions and recycled when sessions close.

---

## Event Log

**File**: `~/.config/wt/events.jsonl`

JSONL format, one event per line:

```json
{"ts":"2026-01-19T08:30:00Z","type":"session.created","session":"toast","bead":"myproject-abc"}
{"ts":"2026-01-19T10:45:00Z","type":"session.status","session":"toast","status":"ready"}
{"ts":"2026-01-19T10:50:00Z","type":"session.closed","session":"toast"}
```

### Event Types

| Type | Description |
|------|-------------|
| `session.created` | New session spawned |
| `session.status` | Status changed |
| `session.closed` | Session cleaned up |
| `session.killed` | Session force killed |

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `WT_CONFIG_DIR` | Override config directory |
| `WT_DEBUG` | Enable debug logging |
| `EDITOR` | Editor for `wt config edit` |

### Session Environment

Inside sessions, these are set:

| Variable | Description |
|----------|-------------|
| `BEADS_DIR` | Path to main repo's beads |
| `PORT_OFFSET` | Port offset for this session |
