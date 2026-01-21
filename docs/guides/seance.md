# Seance: Talking to Past Sessions

**Seance** lets you query past Claude sessions. Instead of parsing logs, you can ask natural language questions to sessions that have completed.

## Why Seance?

After a worker completes:

- Where did they put the config files?
- What approach did they take?
- Why did they make certain decisions?
- What edge cases did they handle?

Instead of reading through transcripts, just ask.

## How It Works

wt logs Claude session IDs to the event log. Seance uses Claude's `--resume` feature to fork these sessions and ask questions.

```
┌─────────────────────────────────────┐
│         Original Session            │
│  (completed, read-only)             │
│                                     │
│  ... work history ...               │
│  ... decisions made ...             │
│  ... context built up ...           │
└─────────────────────────────────────┘
                │
                │ fork (--resume)
                ▼
┌─────────────────────────────────────┐
│         Seance Session              │
│  (new conversation branch)          │
│                                     │
│  "Where did you put the nginx       │
│   config?"                          │
│                                     │
│  → "I created it at                 │
│     /etc/nginx/sites-available/..." │
└─────────────────────────────────────┘
```

The original session is unchanged. Seance creates a branch.

## Commands

### List Past Sessions

```bash
wt seance
```

Output:
```
Past Sessions (seance)

     Session             Title                                 Project         Time
────────────────────────────────────────────────────────────────────────────────────
 ⚙️  myproject-toast     Add OAuth authentication flow         myproject       2026-01-20 14:30
 ⚙️  myproject-shadow    Fix login redirect bug                myproject       2026-01-19 10:15
 🏠  hub                                                                       2026-01-20 12:00

⚙️ = Worker session   🏠 = Hub session
```

Sessions are logged when they end via `wt done`, `wt close`, or `wt kill`. Hub sessions are logged on `wt handoff`.

### Interactive Session

Start a conversation with a past session:

```bash
wt seance toast           # Opens in new tmux pane (safe from hub)
wt seance toast --spawn   # Creates new tmux session
```

This opens an interactive Claude session with full context from the original work.

### Resume Hub Sessions

You can also resume past hub sessions:

```bash
wt seance hub --spawn     # Resume most recent hub in new tmux session
```

### One-Shot Query

Ask a single question:

```bash
wt seance toast -p "Where did you put the nginx config?"
```

Returns the answer and exits.

## Use Cases

### Finding Files

```bash
wt seance toast -p "What files did you create or modify?"
```

### Understanding Decisions

```bash
wt seance toast -p "Why did you choose JWT over session cookies?"
```

### Getting Implementation Details

```bash
wt seance toast -p "How does the rate limiting work?"
```

### Debugging Issues

```bash
wt seance toast -p "I'm seeing auth failures - what could cause this?"
```

### Handoff Context

```bash
wt seance toast -p "Summarize what you did and what's left to do"
```

## Best Practices

### 1. Be Specific

❌ "What did you do?"
✅ "What approach did you take for handling expired tokens?"

### 2. Reference the Work

❌ "Where's the config?"
✅ "Where did you put the database connection config?"

### 3. Ask for Reasoning

The session has context you don't. Ask why:

```bash
wt seance toast -p "Why did you add the retry logic to the API client?"
```

### 4. Use for Code Review

Before reviewing a PR:

```bash
wt seance toast -p "What are the key changes I should focus on in my review?"
```

## Session Availability

Sessions are available as long as:

1. The session ID is logged in events
2. Claude's session storage has the data

Check available sessions:

```bash
wt seance
```

## Limitations

- **Read-only**: You can't modify the original session
- **No new actions**: Seance sessions can't write files or run commands
- **Context window**: Very long sessions may have truncated context
- **Session expiry**: Claude sessions may expire after extended periods

## Example Workflow

After a worker completes auth implementation:

```bash
# Check what was done
wt seance toast -p "Give me a summary of the auth implementation"

# Understand the architecture
wt seance toast -p "What's the token refresh flow?"

# Find potential issues
wt seance toast -p "What edge cases did you consider? Any you didn't handle?"

# Get review guidance
wt seance toast -p "What should I test manually?"
```
