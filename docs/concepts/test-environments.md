# Test Environments

wt provides per-session test environment isolation using **port offsets**. This allows multiple sessions to run their own test databases, servers, and services without conflicts.

## The Problem

When running multiple Claude sessions on the same project:

- Session A starts Postgres on port 5432
- Session B tries to start Postgres on port 5432
- **Port conflict!**

## The Solution: Port Offsets

Each session gets a unique **port offset** (1, 2, 3, ...). Sessions use this offset to calculate their ports:

| Session | Port Offset | Postgres | API Server |
|---------|-------------|----------|------------|
| toast | 1 | 15432 | 13000 |
| shadow | 2 | 25432 | 23000 |
| obsidian | 3 | 35432 | 33000 |

## How It Works

### Environment Variable

Each session gets a `PORT_OFFSET` environment variable:

```bash
# Inside session 'toast'
echo $PORT_OFFSET
# 1
```

### Docker Compose Integration

Your `docker-compose.yml` can use the offset:

```yaml
services:
  postgres:
    image: postgres:15
    ports:
      - "${PORT_OFFSET:-0}5432:5432"
    environment:
      POSTGRES_DB: myapp_test
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
```

When `PORT_OFFSET=1`, Postgres maps to port `15432`.

### Application Configuration

Configure your app to use the offset:

```bash
# .env.test
DATABASE_URL=postgres://localhost:${PORT_OFFSET}5432/myapp_test
API_PORT=${PORT_OFFSET}3000
```

## Project Configuration

Configure test environments in your project config:

```bash
wt project config myproject
```

```json
{
  "test_env": {
    "setup": "docker compose up -d",
    "teardown": "docker compose down",
    "port_env": "PORT_OFFSET",
    "health_check": "curl -sf http://localhost:${PORT_OFFSET}3000/health"
  }
}
```

### Options

| Field | Description |
|-------|-------------|
| `setup` | Command to start test services |
| `teardown` | Command to stop test services |
| `port_env` | Environment variable name for port offset |
| `health_check` | Command to verify services are ready |

## Lifecycle

### On Session Create

1. Assign next available port offset
2. Export `PORT_OFFSET` environment variable
3. Run setup command (`docker compose up -d`)
4. Wait for health check to pass
5. Launch Claude Code

### On Session Close

1. Run teardown command (`docker compose down`)
2. Release port offset for reuse

## Example Setup

### docker-compose.yml

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15
    ports:
      - "${PORT_OFFSET:-0}5432:5432"
    environment:
      POSTGRES_DB: myapp_test
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres

  redis:
    image: redis:7
    ports:
      - "${PORT_OFFSET:-0}6379:6379"

  api:
    build: .
    ports:
      - "${PORT_OFFSET:-0}3000:3000"
    environment:
      DATABASE_URL: postgres://postgres:postgres@postgres:5432/myapp_test
      REDIS_URL: redis://redis:6379
    depends_on:
      - postgres
      - redis
```

### Project Config

```json
{
  "name": "myproject",
  "test_env": {
    "setup": "docker compose up -d --wait",
    "teardown": "docker compose down -v",
    "health_check": "docker compose exec -T api curl -sf localhost:3000/health"
  }
}
```

## Manual Port Management

Check a session's port offset:

```bash
wt status
# Port offset: 1
```

Calculate your ports:

```bash
# Pattern: ${PORT_OFFSET}<base_port>
# Postgres: ${PORT_OFFSET}5432 → 15432, 25432, 35432, ...
# Redis:    ${PORT_OFFSET}6379 → 16379, 26379, 36379, ...
# API:      ${PORT_OFFSET}3000 → 13000, 23000, 33000, ...
```

## Hooks

Run additional commands on session lifecycle:

```json
{
  "hooks": {
    "on_create": [
      "npm install",
      "npm run db:migrate"
    ],
    "on_close": [
      "docker compose down -v"
    ]
  }
}
```
