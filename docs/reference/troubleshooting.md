# Troubleshooting

Common issues and solutions.

## Diagnostic Tools

### wt doctor

Run diagnostics to check your setup:

```bash
wt doctor
```

This checks:

- Git version and worktree support
- Tmux version
- Beads installation
- Configuration validity
- Project registrations

### Event Log

Check recent events for errors:

```bash
wt events -n 20
wt events --since 1h
```

---

## Installation Issues

### macOS Gatekeeper Blocks Binary

**Symptom**: "wt cannot be opened because it is from an unidentified developer"

**Solution**:

```bash
xattr -d com.apple.quarantine $(which wt)
```

Or use the npm installation which bypasses Gatekeeper:

```bash
npm install -g @worktree/wt
```

### Command Not Found

**Symptom**: `wt: command not found`

**Solution**: Ensure the binary is in your PATH:

```bash
# Check where it's installed
which wt

# If using Go install, add GOPATH/bin to PATH
export PATH=$PATH:$(go env GOPATH)/bin

# If using npm global install
export PATH=$PATH:$(npm config get prefix)/bin
```

---

## Session Issues

### Session Won't Start

**Symptom**: `wt new` fails with error

**Possible causes**:

1. **Worktree directory exists**:
   ```bash
   ls ~/worktrees/
   # Remove stale worktree
   rm -rf ~/worktrees/toast
   git worktree prune
   ```

2. **Branch already exists**:
   ```bash
   git branch -D myproject-abc123
   ```

3. **Tmux session exists**:
   ```bash
   tmux kill-session -t toast
   ```

### Session Shows Wrong Status

**Symptom**: Session stuck in "working" but actually idle

**Solution**: Signal the correct status:

```bash
wt signal idle
```

Or check idle detection settings in project config.

### Can't Switch to Session

**Symptom**: `wt toast` fails

**Possible causes**:

1. **Tmux session doesn't exist**:
   ```bash
   tmux ls
   # If missing, session state is stale
   wt kill toast
   ```

2. **Not in tmux**:
   ```bash
   # wt switch requires being in tmux
   tmux attach
   wt toast
   ```

---

## Worktree Issues

### Worktree Creation Fails

**Symptom**: "fatal: 'toast' is already checked out"

**Solution**:

```bash
# List worktrees
git worktree list

# Remove stale references
git worktree prune

# Or manually remove
git worktree remove ~/worktrees/toast --force
```

### Worktree Out of Sync

**Symptom**: Changes from main not appearing

**Solution**:

```bash
# Inside worktree
git fetch origin
git rebase origin/main
```

### Can't Delete Worktree

**Symptom**: "fatal: cannot remove with uncommitted changes"

**Solution**:

```bash
# Force remove
git worktree remove ~/worktrees/toast --force

# Or clean first
cd ~/worktrees/toast
git reset --hard
git clean -fd
cd ~
git worktree remove ~/worktrees/toast
```

---

## Beads Issues

### bd Commands Fail in Session

**Symptom**: `bd list` shows wrong beads or fails

**Solution**: Check BEADS_DIR:

```bash
echo $BEADS_DIR
# Should point to main repo's .beads directory

# If wrong, session state may be corrupted
wt status
```

### Bead Status Not Updating

**Symptom**: Bead stays in wrong status after `wt done`

**Solution**:

```bash
# Manually update
bd update myproject-abc --status=awaiting_review

# Sync with remote
bd sync
```

---

## Port Conflicts

### Port Already in Use

**Symptom**: Services fail to start, "address already in use"

**Solution**:

1. Check what's using the port:
   ```bash
   lsof -i :15432
   ```

2. Kill the conflicting process or use different offset:
   ```bash
   # Check your session's offset
   echo $PORT_OFFSET

   # If another session has same offset, state is corrupted
   wt list
   ```

### Port Offset Not Working

**Symptom**: Services binding to wrong ports

**Solution**: Ensure your configs use PORT_OFFSET:

```bash
# Check the variable is set
echo $PORT_OFFSET

# Use in your commands
docker run -p ${PORT_OFFSET}5432:5432 postgres
```

---

## Tmux Issues

### Keybindings Not Working

**Symptom**: `C-b W` doesn't open session picker

**Solution**:

```bash
# Re-add keybindings
wt keys >> ~/.tmux.conf
tmux source-file ~/.tmux.conf

# Verify they're loaded
tmux list-keys | grep wt
```

### Session Picker Fails

**Symptom**: Picker shows error or crashes

**Solution**: Ensure fzf is installed:

```bash
# Install fzf
brew install fzf  # macOS
apt install fzf   # Ubuntu

# Or use numbered fallback
wt pick  # Works without fzf
```

---

## Configuration Issues

### Config Not Loading

**Symptom**: Settings not being applied

**Solution**:

```bash
# Check config location
echo $WT_CONFIG_DIR  # Should be empty or valid path

# Verify config is valid JSON
cat ~/.config/wt/config.json | jq .

# Re-initialize
wt config init
```

### Project Not Found

**Symptom**: "project 'myproject' not found"

**Solution**:

```bash
# List registered projects
wt projects

# Re-register
wt project add myproject ~/code/myproject
```

---

## Getting Help

If you're still stuck:

1. **Check events**: `wt events -n 50`
2. **Enable debug**: `WT_DEBUG=1 wt <command>`
3. **Check GitHub issues**: [github.com/badri/wt/issues](https://github.com/badri/wt/issues)
4. **Open a new issue** with:
   - Output of `wt doctor`
   - Output of `wt events -n 20`
   - Command you ran and error message
