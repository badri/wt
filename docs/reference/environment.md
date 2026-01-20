# Environment Variables

wt uses and sets various environment variables.

## wt Configuration

These variables configure wt behavior:

| Variable | Description | Default |
|----------|-------------|---------|
| `WT_CONFIG_DIR` | Override config directory | `~/.config/wt` |
| `WT_DEBUG` | Enable debug logging | (unset) |
| `EDITOR` | Editor for config editing | `vim` |

### WT_CONFIG_DIR

Override where wt stores configuration:

```bash
export WT_CONFIG_DIR=/custom/path
wt config show  # Uses /custom/path/config.json
```

### WT_DEBUG

Enable verbose debug output:

```bash
export WT_DEBUG=1
wt new myproject-abc  # Shows debug info
```

---

## Session Environment

These variables are set inside each worker session:

### BEADS_DIR

Path to the main repository's beads directory.

```bash
echo $BEADS_DIR
# /Users/you/myproject/.beads
```

This allows `bd` commands in the worktree to use the main repo's beads:

```bash
# Inside worktree ~/worktrees/toast/
bd list           # Shows beads from /Users/you/myproject/.beads
bd show abc123    # Works correctly
```

### PORT_OFFSET

Sequential number for port isolation.

```bash
echo $PORT_OFFSET
# 1
```

Use in service configurations:

```bash
# Database
DATABASE_URL=postgres://localhost:${PORT_OFFSET}5432/mydb

# API server
API_PORT=${PORT_OFFSET}3000

# Redis
REDIS_URL=redis://localhost:${PORT_OFFSET}6379
```

Port calculation pattern:

| Service | Base Port | Session 1 | Session 2 | Session 3 |
|---------|-----------|-----------|-----------|-----------|
| Postgres | 5432 | 15432 | 25432 | 35432 |
| Redis | 6379 | 16379 | 26379 | 36379 |
| API | 3000 | 13000 | 23000 | 33000 |

---

## Using Environment Variables

### In Docker Compose

```yaml
# docker-compose.yml
services:
  postgres:
    image: postgres:15
    ports:
      - "${PORT_OFFSET:-0}5432:5432"

  api:
    build: .
    ports:
      - "${PORT_OFFSET:-0}3000:3000"
    environment:
      DATABASE_URL: postgres://postgres:postgres@postgres:5432/mydb
```

### In Shell Scripts

```bash
#!/bin/bash
# start-services.sh

export DB_PORT="${PORT_OFFSET}5432"
export API_PORT="${PORT_OFFSET}3000"

docker run -d -p $DB_PORT:5432 postgres:15
./api-server --port $API_PORT
```

### In .env Files

```bash
# .env.local
DATABASE_URL=postgres://localhost:${PORT_OFFSET}5432/mydb
REDIS_URL=redis://localhost:${PORT_OFFSET}6379
API_URL=http://localhost:${PORT_OFFSET}3000
```

### In Application Code

```javascript
// config.js
const portOffset = process.env.PORT_OFFSET || '0';
const config = {
  database: {
    port: parseInt(`${portOffset}5432`),
  },
  redis: {
    port: parseInt(`${portOffset}6379`),
  },
  server: {
    port: parseInt(`${portOffset}3000`),
  },
};
```

```python
# config.py
import os

port_offset = os.environ.get('PORT_OFFSET', '0')

DATABASE_URL = f"postgres://localhost:{port_offset}5432/mydb"
REDIS_URL = f"redis://localhost:{port_offset}6379"
API_PORT = int(f"{port_offset}3000")
```

---

## Tmux Environment

Inside tmux sessions, these are also available:

| Variable | Description |
|----------|-------------|
| `TMUX` | Tmux socket path |
| `TMUX_PANE` | Current pane ID |

---

## Checking Environment

Inside a session:

```bash
# Show all wt-related variables
env | grep -E 'BEADS_DIR|PORT_OFFSET|WT_'

# Check session status
wt status
```

From the hub:

```bash
# See session's port offset
wt list --verbose
```
