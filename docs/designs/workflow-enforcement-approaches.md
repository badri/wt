# Workflow Enforcement: Approaches Compared

**Date:** 2026-02-02  
**Status:** Design exploration  
**Problem:** Autonomous coding agents fail due to lack of workflow discipline

---

## Background

wt provides workspace orchestration (worktrees, sessions, auto mode), but agents still fail because:
- No checkpoints (write 200 lines before testing → fail at end → waste run)
- Ambiguity black holes (guess wrong instead of pausing to ask)
- Architecture blindness (see files, don't understand patterns)
- No backtracking (retry same approach 5x instead of rolling back)
- Scope creep (no discipline to park P2 work)
- Decomposition tax (humans break tasks naturally, agents need it enforced)

**Real failure case:** "Notes CSV import" bead spawned via `wt auto` → agent wandered into edge cases, guessed at ambiguity (dedupe strategy?), didn't test incrementally → timeout or incomplete/broken feature.

**The gap:** wt is a *session orchestrator*, not a *workflow enforcer*. You need both. It's like giving someone a clean desk and tools but no checklist.

---

## Three Approaches

### Approach 1: Pure Skill (Guidance Only)

**What it is:**
- Add `wt-enforce.md` skill to Claude Code
- Teaches Claude the checkpoint pattern, pause workflow, test-first discipline
- No changes to wt codebase

**Implementation:**
```markdown
# wt-enforce.md

When working on a bead:

1. **Break into checkpoints** (3-5 per bead)
   - Each checkpoint = one function/endpoint/migration
   - Define test for each checkpoint before starting

2. **Test after each checkpoint**
   - Write code → run test → commit if green → proceed
   - Don't write 200 lines before testing

3. **Pause instead of guessing**
   - If ambiguous (e.g., "should I dedupe on X or Y?"), signal:
     `wt pause --reason "clarification needed: X or Y?"`
   - Wait for human answer

4. **Rollback when stuck**
   - If stuck for 3 attempts, rollback last commit and try different approach
   - Don't retry same thing 5 times

5. **Park P2 work**
   - If you hit scope creep (e.g., "what about error handling?"), create sub-bead
   - Don't chase completeness
```

**Pros:**
- Zero maintenance (no fork, no code changes)
- Easy to distribute (just a markdown file)
- Works with existing wt today
- Can iterate quickly (just edit the skill)

**Cons:**
- **No enforcement** — Claude can ignore the skill
- Still hits ambiguity black holes (skill can suggest `wt pause`, but wt doesn't have that command)
- Can't gate commits on test passing (wt has no hook for that)
- Relies on Claude's discipline (which is the original problem)

**Verdict:** Better than nothing, but not enough. Guidance alone doesn't fix the core issue.

---

### Approach 2: Hard Fork (Full Enforcement)

**What it is:**
- Fork wt repo → `wt-enforce`
- Add checkpoint validation, pause/resume, test gates, architecture context, dependency checks
- All features built into the tool

**Implementation:**

New bead metadata:
```json
{
  "id": "gw-backend-csv-import",
  "acceptance_criteria": [...],
  "checkpoints": [
    {"id": "schema-defined", "test": "curl returns 501"},
    {"id": "parser-works", "test": "pytest test_csv_parser.py"}
  ],
  "requires_test": true,
  "architecture_context": {...}
}
```

New commands:
- `wt checkpoint <id>` — signal completion, validate test, commit
- `wt pause --reason "<question>"` — pause session, notify user
- `wt resume --answer "<response>"` — resume with answer
- `wt stuck` — signal stuck, offer rollback/pause/skip
- `wt audit --strict` — reject beads without acceptance criteria

Modified auto mode:
- Checkpoint validation loop (block until `wt checkpoint` called + test passes)
- Pause/resume flow
- Stuck detection with rollback

**Pros:**
- **Hard enforcement** — can't bypass checkpoints, test gates, etc.
- Guarantees the discipline
- Full control over features
- Can move fast (no upstream dependency)

**Cons:**
- **Maintenance overhead** — keep fork in sync with upstream wt
- Fragmentation — wt users won't benefit unless they switch
- Not upstreamable if too opinionated
- More complex to distribute (users install fork, not original wt)

**Verdict:** Safest for "actually works" goal, but maintenance burden is real. Only worth it if upstream won't accept PRs.

---

### Approach 3: Hybrid (Skill + MCP Server)

**What it is:**
- **Skill:** Teach Claude the checkpoint pattern, pause workflow, test-first discipline
- **MCP server:** Hub runs `wt-mcp` server, workers connect to it for structured control
- **No wt code changes** — MCP server is separate component

**Implementation:**

Hub runs MCP server:
```python
# wt-mcp server (localhost:8765)

@server.tool()
async def report_checkpoint(session_id: str, checkpoint_id: str, test_result: str):
    # Validate test passed
    # Commit work
    # Send next checkpoint prompt

@server.tool()
async def request_pause(session_id: str, reason: str, options: list[str]):
    # Notify user
    # Wait for resume

@server.tool()
async def signal_stuck(session_id: str, attempts: int, last_error: str):
    # Offer: rollback, pause, skip

@server.tool()
async def get_bead_context(session_id: str):
    # Return acceptance criteria, checkpoints, architecture context
```

Worker calls tools:
```python
# Worker (Claude Code + wt-mcp client)

# At start
context = await get_bead_context()

# After each checkpoint
await report_checkpoint(checkpoint_id="schema-defined", test_result="curl returns 501")

# When ambiguous
await request_pause(reason="Should dedupe on X or Y?", options=["X", "Y"])

# When stuck
await signal_stuck(attempts=3, last_error="DB constraint violation")
```

**Pros:**
- **Hard enforcement** — hub won't proceed without checkpoint signal
- **No fork to maintain** — MCP server is separate, wt unchanged
- **Bidirectional** — worker can query hub state, signal back
- **Programmatic** — hub can enforce workflow
- **Upstreamable** — could contribute MCP integration to wt later
- **Best of both** — tmux for persistence, MCP for control

**Cons:**
- Adds new component (MCP server to run)
- Requires MCP client support in Claude Code (or wrapper script)
- More complex than pure skill
- Skill still needed (teach workers when/how to call tools)

**Verdict:** Best of all worlds. Structured control without fork maintenance. Can ship independently of wt upstream.

---

### Approach 4: wt Core PRs (Upstreamable, Optional)

**What it is:**
- Submit PRs to wt with enforcement features as **opt-in flags**
- All changes upstreamable (backward-compatible, not breaking)
- If PRs rejected, maintain feature branch

**Implementation:**

PR 1: Checkpoint support
```bash
# Bead metadata (optional)
bd create --checkpoints "schema,parser,e2e"

# Command (opt-in)
wt new <bead> --enforce-checkpoints
# → Worker must call `wt checkpoint <id>` to proceed
```

PR 2: Pause/Resume
```bash
wt pause --reason "Need clarification: X or Y?"
# → Hub notifies user, session paused

wt resume --answer "Use X"
# → Inject answer, resume worker
```

PR 3: Test-first enforcement
```bash
wt commit --require-test
# → Blocks commit if test fails

# Config (per-project)
require_test: true
```

PR 4: Strict audit
```bash
wt audit --strict
# → Rejects beads without acceptance criteria or checkpoints
```

**Pros:**
- **No fork** — contribute to upstream, everyone benefits
- **Opt-in** — doesn't break existing workflows
- **Upstreamable** — designed for acceptance by wt maintainer
- **Lower maintenance** — just track upstream wt

**Cons:**
- **Depends on wt maintainer** — PRs might be rejected or take time
- **Can't control roadmap** — need buy-in for each feature
- **Architecture context might be too opinionated** for upstream

**Fallback:** If PRs rejected, maintain feature branch and rebase periodically.

**Verdict:** Best if wt maintainer is receptive. Need to check GitHub issues/PRs for maintainer activity and contribution guidelines.

---

## Recommended Path

### Phase 1: Pure Skill (Week 1)
- Write `wt-enforce.md` skill — teaches checkpoint pattern, pause workflow, test-first
- Test with existing wt, manual discipline
- Document what works vs. what fails
- **Expected:** Better than nothing, but Claude still fails sometimes (no hard gates)

### Phase 2: MCP Server (Week 2-3)
- Build minimal `wt-mcp` server with core tools: `report_checkpoint`, `request_pause`, `get_bead_context`
- Hub runs server, workers connect
- Test checkpoint validation loop + pause/resume
- **Expected:** Hard enforcement works, autonomous coding success rate goes up

### Phase 3: Add More Tools (Week 3-4)
- Add `signal_stuck`, `get_architecture_context`
- Implement rollback mechanism
- Architecture context scanner (DB schema, routes, services)
- **Expected:** Stuck detection + architecture awareness reduces failures further

### Phase 4: Upstream or Branch (Optional)
- If wt maintainer is receptive: extract MCP integration as PR to wt
- If not: keep `wt-mcp` as separate tool (works alongside wt)
- **Expected:** Wider adoption if upstreamed, fine as standalone otherwise

---

## Comparison Matrix

| Approach | Enforcement | Maintenance | Upstreamable | Complexity |
|----------|-------------|-------------|--------------|------------|
| Pure Skill | ❌ Guidance only | ✅ Zero | ✅ Just a file | ⭐ Simple |
| Hard Fork | ✅ Built-in | ❌ High (sync fork) | ❌ Opinionated | ⭐⭐⭐ Complex |
| MCP Server | ✅ Structured | ✅ Low (separate) | ✅ Could PR later | ⭐⭐ Moderate |
| wt Core PRs | ✅ Opt-in flags | ✅ Upstream tracks | ✅ Designed for it | ⭐⭐ Moderate |

---

## Decision

**Start with Hybrid: Skill + MCP Server**

**Why:**
1. **Skill first** — test guidance-only approach, identify what still fails
2. **MCP for enforcement** — add structured control without forking wt
3. **No upstream dependency** — can ship immediately
4. **Can upstream later** — if wt maintainer wants it

**Keep architecture context separate:**
- Build as standalone tool: `wt-context <project>` → generates `ARCHITECTURE.md`
- Scans DB schema, routes, services → injects before worker spawn
- Less coupling, easier to maintain
- Can be used with or without wt

**Fallback:**
- If MCP proves too complex, try wt core PRs (opt-in flags)
- If PRs rejected, maintain feature branch
- **Don't hard fork unless necessary** — maintenance burden too high

---

## Success Metrics

1. **"Notes CSV import" epic** → spawn via `wt auto --epic csv-import` → wake up 2 hours later → feature shipped, tests green, PR open
2. **Ambiguity pause** → agent hits unclear requirement → pauses → you answer in 30 seconds → resumes → finishes correctly
3. **Stuck retry** → agent hits integration failure → rollback → tries different approach → succeeds
4. **Zero wasted runs** → no more "agent wrote 200 lines, then failed, whole session wasted"

---

## Next Steps

1. Write `wt-enforce.md` skill (Phase 1)
2. Test with existing wt on real bead (notes CSV import redux?)
3. Document failures → identify what needs MCP enforcement
4. Build minimal `wt-mcp` server (Phase 2)
5. Spike checkpoint validation loop
6. Ship Phase 1-2 first (biggest bang for buck)
7. Iterate on stuck detection, architecture context based on real usage

---

**The insight:** wt handles workspace, skill provides guidance, MCP enforces workflow. Together = autonomous coding that actually works.
