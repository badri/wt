# MCP Integration with wt

## Overview

wt (worktree manager) and mcpx (MCP CLI bridge) are complementary tools that work together without tight coupling.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    wt session                           │
│  ┌─────────────────────────────────────────────────┐   │
│  │  tmux session: "toast"                          │   │
│  │  cwd: ~/worktrees/toast/                        │   │
│  │  env: BEADS_DIR, PORT_OFFSET                    │   │
│  │                                                 │   │
│  │  ┌─────────────┐      ┌──────────────────────┐ │   │
│  │  │ mcp.json    │ ───▶ │ mcpx (reads from cwd)│ │   │
│  │  │ (project)   │      └──────────────────────┘ │   │
│  │  └─────────────┘               │               │   │
│  └────────────────────────────────│───────────────┘   │
└───────────────────────────────────│───────────────────┘
                                    ▼
                          ┌──────────────────┐
                          │  mcpx daemon     │
                          │  (centralized)   │
                          └──────────────────┘
                                    │
                                    ▼
                          ┌──────────────────┐
                          │  MCP Servers     │
                          │  (Supabase, etc) │
                          └──────────────────┘
```

## Design Decisions

### Separation of Concerns

| Tool | Responsibility |
|------|----------------|
| wt | Worktree isolation, tmux sessions, session env vars |
| mcpx | MCP protocol bridge, daemon lifecycle, config resolution |

### Why No Tight Integration

1. **Natural isolation** - wt creates isolated worktrees; mcpx respects cwd for config
2. **Independent evolution** - Each tool can evolve without breaking the other
3. **Simplicity** - No coordination required between tools
4. **Flexibility** - Users can use mcpx without wt, or wt without mcpx

## Config Resolution (mcpx feature)

mcpx should resolve config in this order:

```
1. ./mcp.json           (project-local, checked into repo)
2. ~/.mcpx/servers.json (global fallback)
```

This allows:
- Projects to define their own MCP servers (Supabase project, BetterStack team, etc.)
- Team members to share MCP config via version control
- Personal/shared servers available globally as fallback

## Example Workflow

```bash
# Terminal 1: Start mcpx daemon (once, globally)
mcpx --daemon

# Terminal 2: Create wt session for a bead
wt new myproject-abc123

# Inside the tmux session (~/worktrees/toast/):
# - Project has its own mcp.json with Supabase config
# - mcpx reads ./mcp.json automatically
# - Claude Code can query project's database via mcpx

# Ask Claude: "Show me users created today"
# Claude invokes: mcpx --query supabase execute_sql '{"query": "..."}'
# mcpx reads ./mcp.json, finds supabase config, executes query
```

## Project mcp.json Example

```json
{
  "servers": {
    "supabase": {
      "url": "https://mcp.supabase.com/mcp?project_ref=my-project&read_only=true"
    },
    "betterstack": {
      "url": "https://mcp.betterstack.com"
    }
  }
}
```

## Future Considerations

### Optional wt Enhancement

If projects need non-standard config paths, wt could export `MCPX_CONFIG` env var:

```json
{
  "name": "myproject",
  "mcp_config": "./config/mcp.json"
}
```

wt would then set `MCPX_CONFIG=./config/mcp.json` in the tmux session.

**Current recommendation:** Use `./mcp.json` convention instead. Simpler, no wt changes needed.

### mcpx Feature Request

For this integration to work, mcpx needs:
- [ ] Support reading `./mcp.json` from current working directory
- [ ] Fall back to `~/.mcpx/servers.json` if local config not found
- [ ] Optionally support `MCPX_CONFIG` env var for custom paths

## Summary

wt and mcpx complement each other through convention, not code:
- wt creates isolated worktrees (each with its own cwd)
- mcpx reads config from cwd (respects isolation automatically)
- No implementation needed in wt for MCP support
