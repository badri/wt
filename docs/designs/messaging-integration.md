# Messaging Integration Design

> Design doc for wt-4yq: Map messaging use cases to wt workflow

## Overview

Replace tmux polling with structured messaging using `internal/msg` (SQLite store from wt-h1z). The orchestrator and workers communicate through typed messages (TASK/DONE/STUCK/PROGRESS) instead of polling child process existence.

## Current State

The orchestrator (`processEpic`) uses three mechanisms today:
1. **tmux paste-buffer** — sends prompts to workers
2. **pgrep polling** — checks if Claude is still running (every 10s)
3. **wt signal bead-done** — worker signals completion via CLI

Problems:
- Polling is coarse (10s intervals) and can miss fast failures
- No structured payload on completion (just "success"/"timeout"/"stopped")
- No way for workers to report progress mid-bead
- No way for orchestrator to ask worker questions
- Crash recovery relies on reconstructing state from EpicState JSON, not from message history

## Message Type Catalog

### Core Messages

| Subject | From | To | Body | Thread | Ack Required |
|---------|------|-----|------|--------|-------------|
| TASK | orchestrator | worker | `{"bead_id": "...", "title": "...", "prompt": "...", "bead_num": 1, "total": 5}` | epic ID | Yes |
| DONE | worker | orchestrator | `{"bead_id": "...", "commit_hash": "...", "summary": "..."}` | epic ID | Yes |
| STUCK | worker | orchestrator | `{"bead_id": "...", "reason": "...", "needs": "guidance|dependency|abort"}` | epic ID | Yes |
| PROGRESS | worker | orchestrator | `{"bead_id": "...", "percent": 50, "status": "running tests"}` | epic ID | No |

### Body Schemas

```go
// TaskBody is sent by the orchestrator to assign a bead.
type TaskBody struct {
    BeadID   string   `json:"bead_id"`
    Title    string   `json:"title"`
    Prompt   string   `json:"prompt"`
    BeadNum  int      `json:"bead_num"`
    Total    int      `json:"total"`
    Prior    []string `json:"prior_summaries,omitempty"` // summaries from completed beads
}

// DoneBody is sent by the worker on completion.
type DoneBody struct {
    BeadID     string `json:"bead_id"`
    CommitHash string `json:"commit_hash"`
    Summary    string `json:"summary"`
}

// StuckBody is sent by the worker when it can't proceed.
type StuckBody struct {
    BeadID string `json:"bead_id"`
    Reason string `json:"reason"`
    Needs  string `json:"needs"` // "guidance", "dependency", "abort"
}

// ProgressBody is sent periodically by the worker.
type ProgressBody struct {
    BeadID  string `json:"bead_id"`
    Percent int    `json:"percent,omitempty"`
    Status  string `json:"status"`
}
```

## Threading Model

- **Thread ID** = epic ID (e.g., `wt-doc-epic`)
- All messages for one epic run share a thread
- Within a thread, messages are ordered by ID (auto-increment)
- One-shot messages (not conversations) — orchestrator sends TASK, worker sends DONE/STUCK
- PROGRESS messages are informational, no reply expected

## Use Case Mappings

### 1. Sequential Auto (current flow, messaging replacement)

**Today:**
```
orchestrator → [tmux paste-buffer] → worker
orchestrator ← [pgrep poll every 10s] ← worker exits
orchestrator ← [wt signal bead-done "summary"] ← worker
```

**With messaging:**
```
orchestrator → Send(TASK, to=worker, body={bead, prompt}) → worker
worker reads TASK via Recv(), acks it
worker does work...
worker → Send(DONE, to=orchestrator, body={commit, summary})
orchestrator reads DONE via Recv(), acks it, assigns next bead
```

**Integration in processEpic():**
```go
// Replace runClaudeInSession() polling with:
store.Send(&msg.Message{Subject: "TASK", To: sessionName, Body: taskJSON, ThreadID: epicID})

// Replace isSessionActive() polling with:
// Poll store.Recv("orchestrator") for DONE/STUCK messages
for {
    msgs, _ := store.Recv("orchestrator")
    for _, m := range msgs {
        switch m.Subject {
        case "DONE":
            store.Ack(m.ID)
            // proceed to next bead
        case "STUCK":
            store.Ack(m.ID)
            // handle failure
        case "PROGRESS":
            // log/display, no ack needed
        }
    }
    time.Sleep(2 * time.Second)
}
```

### 2. Parallel Auto (future: multiple workers)

**Messaging enables this without tmux:**
```
orchestrator → TASK(bead-1) → worker-1
orchestrator → TASK(bead-2) → worker-2
worker-1 → DONE(bead-1) → orchestrator
worker-2 → STUCK(bead-2) → orchestrator
```

Each worker has a unique identity (e.g., session name). The orchestrator fans out TASKs and collects DONE/STUCK. No polling needed — just `Recv("orchestrator")`.

**File reservation** (for conflict avoidance): workers send PROGRESS messages listing files they're editing. Orchestrator tracks these to avoid assigning overlapping beads.

### 3. Hub-Worker Dialogue

Not covered by current messaging primitives. Would need a new message subject:
- `QUERY` — hub asks worker a question
- `REPLY` — worker responds

Deferred to a future bead. Current hub is a UX session, not an orchestrator.

### 4. Worker-Worker Coordination

Not needed for sequential auto. For parallel auto, the orchestrator mediates all coordination — workers don't talk directly. This is simpler and avoids deadlocks.

### 5. Audit Trail

Every message is persisted in SQLite with timestamps. The `messages` table serves as a complete audit log:
```sql
SELECT * FROM messages WHERE thread_id = 'epic-1' ORDER BY id;
```

No additional logging needed — the store IS the audit trail.

### 6. Crash Recovery

On orchestrator restart:
```go
// Find unacked messages (work that was assigned but never completed)
unacked, _ := store.Unacked()
for _, m := range unacked {
    if m.Subject == "TASK" && m.To != "orchestrator" {
        // Bead was assigned but worker never responded — retry or mark failed
    }
    if m.Subject == "DONE" && m.To == "orchestrator" {
        // Worker finished but orchestrator never processed — process now
    }
}
```

This replaces the current EpicState-based recovery with message-based recovery. EpicState can be reconstructed from the message history.

## Migration Path

### Phase 1: Dual-write (non-breaking)
- `processEpic()` sends TASK messages AND uses tmux paste-buffer
- `wt signal bead-done` sends DONE message AND writes EpicState
- Orchestrator still polls tmux, but also logs to message store
- Validates message flow without changing behavior

### Phase 2: Message-based detection
- Orchestrator polls `store.Recv()` instead of `isSessionActive()`
- Worker sends DONE/STUCK via `wt msg send` (called from Claude skill/hook)
- tmux still used for session management, but not for signaling
- EpicState still maintained for backwards compat

### Phase 3: Full messaging
- Remove tmux polling entirely
- Workers read TASK from store, send DONE/STUCK to store
- Orchestrator is a pure message consumer
- Enable parallel workers (multiple Recv targets)

## Worker-Side Integration

Workers need to send messages. Two options:

**Option A: CLI hook (recommended for Phase 2)**
Worker's Claude session has a hook that runs `wt msg send` on bead completion:
```bash
# In .claude/hooks/post-bead.sh
wt msg send --to orchestrator --subject DONE --body "{\"bead_id\":\"$BEAD_ID\",\"summary\":\"$SUMMARY\"}" --thread "$EPIC_ID"
```

**Option B: Skill-based (Phase 3)**
The wt skill instructs Claude to call `wt msg send` as part of the bead completion flow, replacing `wt signal bead-done`.

## Files to Modify (Implementation)

| File | Change |
|------|--------|
| `internal/auto/auto.go` | Add store field to Runner, send TASK on bead start, recv DONE/STUCK |
| `internal/msg/store.go` | Add body schema types (TaskBody, DoneBody, etc.) |
| `cmd/wt/session_commands.go` | Update `cmdSignal` to also send DONE message |
| `examples/claude-code-skill/SKILL.md` | Document `wt msg` commands for workers |
