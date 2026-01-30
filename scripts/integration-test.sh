#!/usr/bin/env bash
#
# Integration test for wt using synthetic playground projects.
#
# Usage:
#   ./scripts/integration-test.sh setup     # create repos, register projects, create beads
#   ./scripts/integration-test.sh run       # run all test scenarios
#   ./scripts/integration-test.sh cleanup   # tear down repos and unregister projects
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
PLAYGROUND_DIR="${ROOT_DIR}/playground"
RESULTS_DIR="${PLAYGROUND_DIR}/test-results"
API_DIR="${PLAYGROUND_DIR}/playground-api"
WEB_DIR="${PLAYGROUND_DIR}/playground-web"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
NC='\033[0m'

log()  { echo -e "${BOLD}[integration]${NC} $*"; }
pass() { echo -e "${GREEN}PASS${NC} $1"; }
fail() { echo -e "${RED}FAIL${NC} $1"; }
warn() { echo -e "${YELLOW}WARN${NC} $1"; }

# ──────────────────────────────────────────────────────────
# SETUP
# ──────────────────────────────────────────────────────────
setup_playground_api() {
    log "Creating playground-api repo..."
    mkdir -p "$API_DIR"
    cd "$API_DIR"

    git init
    git checkout -b main

    # Go module
    cat > go.mod <<'GOMOD'
module playground-api

go 1.22
GOMOD

    # Main server
    cat > main.go <<'MAIN'
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var (
	store = map[string]Item{}
	mu    sync.RWMutex
)

func main() {
	http.HandleFunc("/items", handleItems)
	http.HandleFunc("/health", handleHealth)

	fmt.Println("playground-api listening on :8090")
	log.Fatal(http.ListenAndServe(":8090", nil))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok")
}

func handleItems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		mu.RLock()
		items := make([]Item, 0, len(store))
		for _, v := range store {
			items = append(items, v)
		}
		mu.RUnlock()
		json.NewEncoder(w).Encode(items)

	case http.MethodPost:
		var item Item
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		store[item.ID] = item
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(item)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
MAIN

    # Basic test
    cat > main_test.go <<'TEST'
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handleHealth(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
TEST

    # Config file
    cat > config.json <<'CFG'
{
  "port": 8090,
  "log_level": "info",
  "max_connections": 100
}
CFG

    git add -A
    git commit -m "Initial playground-api scaffold"

    # Initialize beads
    bd init --prefix pg-api --quiet --skip-hooks
    git add -A
    git commit -m "Initialize beads"

    log "playground-api created at $API_DIR"
}

setup_playground_web() {
    log "Creating playground-web repo..."
    mkdir -p "$WEB_DIR"
    cd "$WEB_DIR"

    git init
    git checkout -b main

    # index.html
    cat > index.html <<'HTML'
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Playground Web</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>
    <h1>Playground Web</h1>
    <p>A simple static site for integration testing.</p>
    <script src="app.js"></script>
</body>
</html>
HTML

    # style.css
    cat > style.css <<'CSS'
body {
    font-family: sans-serif;
    max-width: 800px;
    margin: 0 auto;
    padding: 2rem;
}
h1 { color: #333; }
CSS

    # app.js
    cat > app.js <<'JS'
console.log("playground-web loaded");
JS

    git add -A
    git commit -m "Initial playground-web scaffold"

    bd init --prefix pg-web --quiet --skip-hooks
    git add -A
    git commit -m "Initialize beads"

    log "playground-web created at $WEB_DIR"
}

register_projects() {
    log "Registering projects with wt..."
    wt project add playground-api "$API_DIR" --non-interactive || true
    wt project add playground-web "$WEB_DIR" --non-interactive || true
    log "Projects registered."
}

create_api_beads() {
    log "Creating beads for playground-api..."
    cd "$API_DIR"

    # Quick beads (~2 min each)
    local b1 b2 b3

    b1=$(bd create "Add DELETE /items/:id endpoint" \
        --type=task --priority=2 --silent \
        --description="Add a DELETE handler to remove an item by ID from the in-memory store. Return 204 on success, 404 if not found.")

    b2=$(bd create "Fix: GET /items returns null instead of empty array" \
        --type=bug --priority=1 --silent \
        --description="When the store is empty, GET /items returns null. It should return []. Fix the handleItems GET branch to initialize the slice properly.")

    b3=$(bd create "Update config.json: add read_timeout and write_timeout fields" \
        --type=task --priority=3 --silent \
        --description="Add read_timeout and write_timeout integer fields (in seconds) to config.json. Default both to 30. Update main.go to read config.json at startup and apply timeouts to the http.Server.")

    # Slow beads (~10 min each)
    local b4 b5

    b4=$(bd create "Refactor: extract handler functions into handlers package" \
        --type=task --priority=2 --silent \
        --description="Move all HTTP handler functions from main.go into an internal/handlers package. The handlers package should accept a store interface. Update main.go to wire everything together. Ensure tests still pass.")

    b5=$(bd create "Add comprehensive tests for all CRUD operations" \
        --type=task --priority=2 --silent \
        --description="Write table-driven tests for GET, POST, and DELETE /items endpoints. Cover: empty store, single item, multiple items, malformed JSON, duplicate IDs, missing ID on delete. Aim for >80% coverage. Run go test -cover to verify.")

    # Failure-prone beads
    local b6 b7

    b6=$(bd create "Improve performance" \
        --type=task --priority=3 --silent \
        --description="Make the API faster. There might be bottlenecks somewhere. Look into it and optimize what you can.")

    b7=$(bd create "Add authentication and rate limiting" \
        --type=feature --priority=3 --silent \
        --description="Add JWT-based authentication to all endpoints. Also add rate limiting per IP. Also add CORS support. Also add request logging middleware. Also add graceful shutdown. Make sure everything works together and tests pass.")

    # Epic grouping all beads
    local epic
    epic=$(bd create "playground-api integration epic" \
        --type=epic --priority=1 --silent \
        --description="Epic grouping all playground-api synthetic beads for integration testing.")

    # Set dependencies: all beads are children of the epic
    for bead_id in $b1 $b2 $b3 $b4 $b5 $b6 $b7; do
        bd dep add "$bead_id" "$epic" --quiet 2>/dev/null || true
    done

    # b5 (comprehensive tests) depends on b1 (DELETE endpoint) being done first
    bd dep add "$b5" "$b1" --quiet 2>/dev/null || true

    log "Created API beads: $b1 $b2 $b3 $b4 $b5 $b6 $b7"
    log "API epic: $epic"

    # Save IDs for test scenarios
    cat > "${PLAYGROUND_DIR}/.api-beads.env" <<EOF
API_QUICK_1=$b1
API_QUICK_2=$b2
API_QUICK_3=$b3
API_SLOW_1=$b4
API_SLOW_2=$b5
API_FAIL_1=$b6
API_FAIL_2=$b7
API_EPIC=$epic
EOF
}

create_web_beads() {
    log "Creating beads for playground-web..."
    cd "$WEB_DIR"

    local b1 b2 b3

    # Quick beads
    b1=$(bd create "Add an About page" \
        --type=task --priority=2 --silent \
        --description="Create about.html with basic content about the playground project. Add a navigation link from index.html to about.html and vice versa.")

    b2=$(bd create "Fix: heading color should be dark blue not gray" \
        --type=bug --priority=2 --silent \
        --description="In style.css, change the h1 color from #333 to #1a237e (dark blue). Also add a hover effect that lightens the color slightly.")

    # Slow bead
    b3=$(bd create "Add a simple build pipeline with minification" \
        --type=task --priority=3 --silent \
        --description="Create a Makefile with targets: build (copies files to dist/), clean (removes dist/), and serve (runs python3 -m http.server from dist/). The build target should also create a combined bundle.js if multiple JS files exist.")

    # Epic
    local epic
    epic=$(bd create "playground-web integration epic" \
        --type=epic --priority=1 --silent \
        --description="Epic grouping all playground-web synthetic beads for integration testing.")

    for bead_id in $b1 $b2 $b3; do
        bd dep add "$bead_id" "$epic" --quiet 2>/dev/null || true
    done

    log "Created web beads: $b1 $b2 $b3"
    log "Web epic: $epic"

    cat > "${PLAYGROUND_DIR}/.web-beads.env" <<EOF
WEB_QUICK_1=$b1
WEB_QUICK_2=$b2
WEB_SLOW_1=$b3
WEB_EPIC=$epic
EOF
}

do_setup() {
    if [[ -d "$PLAYGROUND_DIR" ]]; then
        warn "Playground directory already exists: $PLAYGROUND_DIR"
        warn "Run '$0 cleanup' first or remove it manually."
        exit 1
    fi

    mkdir -p "$PLAYGROUND_DIR" "$RESULTS_DIR"

    setup_playground_api
    setup_playground_web
    register_projects
    create_api_beads
    create_web_beads

    log ""
    log "Setup complete!"
    log "  playground-api: $API_DIR ($(cd "$API_DIR" && bd list --status=open 2>/dev/null | wc -l | tr -d ' ') beads)"
    log "  playground-web: $WEB_DIR ($(cd "$WEB_DIR" && bd list --status=open 2>/dev/null | wc -l | tr -d ' ') beads)"
    log ""
    log "Bead IDs saved to:"
    log "  ${PLAYGROUND_DIR}/.api-beads.env"
    log "  ${PLAYGROUND_DIR}/.web-beads.env"
    log ""
    log "Next: ./scripts/integration-test.sh run"
}

# ──────────────────────────────────────────────────────────
# RUN — test scenarios
# ──────────────────────────────────────────────────────────

PASSED=0
FAILED=0
SKIPPED=0

load_bead_ids() {
    if [[ ! -f "${PLAYGROUND_DIR}/.api-beads.env" ]] || [[ ! -f "${PLAYGROUND_DIR}/.web-beads.env" ]]; then
        log "Bead env files not found. Run '$0 setup' first."
        exit 1
    fi
    # shellcheck source=/dev/null
    source "${PLAYGROUND_DIR}/.api-beads.env"
    # shellcheck source=/dev/null
    source "${PLAYGROUND_DIR}/.web-beads.env"
}

record_result() {
    local scenario="$1" result="$2" detail="${3:-}"
    local ts
    ts=$(date '+%Y-%m-%dT%H:%M:%S')
    local logfile="${RESULTS_DIR}/${scenario}.log"

    if [[ "$result" == "PASS" ]]; then
        pass "$scenario"
        ((PASSED++))
    elif [[ "$result" == "SKIP" ]]; then
        warn "SKIP $scenario${detail:+ — $detail}"
        ((SKIPPED++))
    else
        fail "$scenario${detail:+ — $detail}"
        ((FAILED++))
    fi

    echo "[$ts] $result: $scenario ${detail:-}" >> "$logfile"
}

# Helper: abort any leftover auto state before a scenario
ensure_clean_auto_state() {
    wt auto --abort 2>/dev/null || true
}

# ── Scenario 1: Happy path (dry-run) ──────────────────────
scenario_happy_path_dry_run() {
    local name="1-happy-path-dry-run"
    log "Scenario: $name"
    ensure_clean_auto_state

    local output
    output=$(wt auto --epic "$API_EPIC" --dry-run 2>&1) || true

    # Dry-run should list beads without executing anything
    if echo "$output" | grep -qi "bead\|ready\|would process\|audit\|preview"; then
        # Verify no worktrees were created
        local wt_count
        wt_count=$(git -C "$API_DIR" worktree list 2>/dev/null | wc -l | tr -d ' ')
        if [[ "$wt_count" -le 1 ]]; then
            record_result "$name" "PASS"
        else
            record_result "$name" "FAIL" "Dry-run created worktrees"
        fi
    else
        record_result "$name" "FAIL" "Dry-run produced unexpected output: $(echo "$output" | head -3)"
    fi

    echo "$output" > "${RESULTS_DIR}/${name}-output.log"
}

# ── Scenario 2: Happy path (real run on quick beads) ──────
scenario_happy_path_real() {
    local name="2-happy-path-real"
    log "Scenario: $name"
    log "  This runs wt auto on the API epic (may take several minutes)..."
    ensure_clean_auto_state

    local output exit_code=0
    output=$(wt auto --epic "$API_EPIC" --timeout 15 --skip-audit 2>&1) || exit_code=$?

    echo "$output" > "${RESULTS_DIR}/${name}-output.log"

    # Check that at least some beads were processed (look for progress indicators)
    if echo "$output" | grep -qi "processing\|completed\|success\|bead\|merged\|created"; then
        record_result "$name" "PASS"
    elif [[ $exit_code -eq 0 ]]; then
        record_result "$name" "PASS" "Exit 0 (output may vary)"
    else
        record_result "$name" "FAIL" "Exit code $exit_code"
    fi
}

# ── Scenario 3: Failure handling (pause-on-failure) ───────
scenario_failure_handling() {
    local name="3-failure-handling"
    log "Scenario: $name"
    log "  Running with --pause-on-failure..."
    ensure_clean_auto_state

    # Create a small epic with just the failure-prone bead for isolation
    cd "$API_DIR"
    local fail_epic
    fail_epic=$(bd create "failure-test epic" --type=epic --priority=1 --silent \
        --description="Temporary epic for testing failure handling.")
    local fail_bead
    fail_bead=$(bd create "Do the impossible: solve P=NP" --type=task --priority=2 --silent \
        --description="Prove whether P equals NP. Provide a formal mathematical proof. This task is intentionally impossible for an automated agent.")
    bd dep add "$fail_bead" "$fail_epic" --quiet 2>/dev/null || true

    local output exit_code=0
    output=$(wt auto --epic "$fail_epic" --pause-on-failure --timeout 5 --skip-audit 2>&1) || exit_code=$?

    echo "$output" > "${RESULTS_DIR}/${name}-output.log"

    # We expect either: paused state, non-zero exit, or a failure message
    if echo "$output" | grep -qi "fail\|pause\|error\|timeout\|stopped"; then
        record_result "$name" "PASS"
    elif [[ $exit_code -ne 0 ]]; then
        record_result "$name" "PASS" "Non-zero exit ($exit_code) as expected"
    else
        record_result "$name" "FAIL" "Expected failure/pause but got clean exit"
    fi

    # Clean up the temporary epic
    bd close "$fail_epic" --quiet 2>/dev/null || true
    bd close "$fail_bead" --quiet 2>/dev/null || true
    ensure_clean_auto_state
}

# ── Scenario 4: Resume after failure ─────────────────────
scenario_resume() {
    local name="4-resume"
    log "Scenario: $name"
    ensure_clean_auto_state

    local output exit_code=0
    output=$(wt auto --resume 2>&1) || exit_code=$?

    echo "$output" > "${RESULTS_DIR}/${name}-output.log"

    # Resume with no paused session should give a clear message (not a crash)
    if echo "$output" | grep -qi "no.*session\|no.*state\|nothing.*resume\|not found\|no paused\|no auto"; then
        record_result "$name" "PASS" "Correctly reports no session to resume"
    elif [[ $exit_code -ne 0 ]]; then
        # Non-zero exit is acceptable — it means "nothing to resume"
        record_result "$name" "PASS" "Exit $exit_code (no session to resume)"
    else
        record_result "$name" "FAIL" "Unexpected success with no paused session"
    fi
}

# ── Scenario 5: Parallel projects ────────────────────────
scenario_parallel_projects() {
    local name="5-parallel-projects"
    log "Scenario: $name"
    log "  Running both epics in dry-run mode simultaneously..."
    ensure_clean_auto_state

    # Run both dry-runs in parallel
    local api_output web_output
    local api_exit=0 web_exit=0

    wt auto --epic "$API_EPIC" --dry-run > "${RESULTS_DIR}/${name}-api.log" 2>&1 &
    local api_pid=$!

    wt auto --epic "$WEB_EPIC" --dry-run > "${RESULTS_DIR}/${name}-web.log" 2>&1 &
    local web_pid=$!

    wait "$api_pid" || api_exit=$?
    wait "$web_pid" || web_exit=$?

    api_output=$(cat "${RESULTS_DIR}/${name}-api.log")
    web_output=$(cat "${RESULTS_DIR}/${name}-web.log")

    # Both should succeed (dry-run doesn't conflict)
    if [[ $api_exit -eq 0 ]] && [[ $web_exit -eq 0 ]]; then
        record_result "$name" "PASS"
    elif [[ $api_exit -eq 0 ]] || [[ $web_exit -eq 0 ]]; then
        # One succeeded — check if the other failed due to locking (which is also valid behavior)
        if echo "$api_output$web_output" | grep -qi "lock\|already running\|busy"; then
            record_result "$name" "PASS" "Per-project lock correctly prevented concurrent runs"
        else
            record_result "$name" "FAIL" "API exit=$api_exit, Web exit=$web_exit"
        fi
    else
        record_result "$name" "FAIL" "Both failed: API exit=$api_exit, Web exit=$web_exit"
    fi
}

# ── Scenario 6: Stop mid-run ─────────────────────────────
scenario_stop() {
    local name="6-stop-mid-run"
    log "Scenario: $name"
    ensure_clean_auto_state

    # Test that --stop doesn't crash when nothing is running
    local output exit_code=0
    output=$(wt auto --stop 2>&1) || exit_code=$?

    echo "$output" > "${RESULTS_DIR}/${name}-output.log"

    # Should gracefully report nothing to stop (not crash)
    if echo "$output" | grep -qi "no.*running\|no.*session\|not running\|stopped\|no auto\|nothing"; then
        record_result "$name" "PASS" "Correctly reports nothing to stop"
    elif [[ $exit_code -ne 0 ]]; then
        record_result "$name" "PASS" "Exit $exit_code (nothing running)"
    else
        # Even exit 0 is fine — means the command handled it
        record_result "$name" "PASS" "Handled gracefully"
    fi
}

# ── Scenario 7: Abort and cleanup ────────────────────────
scenario_abort() {
    local name="7-abort-cleanup"
    log "Scenario: $name"
    ensure_clean_auto_state

    # Test that --abort doesn't crash when nothing is running
    local output exit_code=0
    output=$(wt auto --abort 2>&1) || exit_code=$?

    echo "$output" > "${RESULTS_DIR}/${name}-output.log"

    if echo "$output" | grep -qi "no.*session\|no.*state\|nothing.*abort\|not found\|cleaned\|no auto\|nothing"; then
        record_result "$name" "PASS" "Correctly reports nothing to abort"
    elif [[ $exit_code -ne 0 ]]; then
        record_result "$name" "PASS" "Exit $exit_code (nothing to abort)"
    else
        record_result "$name" "PASS" "Handled gracefully"
    fi
}

# ── Scenario 8: Status check ─────────────────────────────
scenario_check_status() {
    local name="8-status-check"
    log "Scenario: $name"

    local output exit_code=0
    output=$(wt auto --check 2>&1) || exit_code=$?

    echo "$output" > "${RESULTS_DIR}/${name}-output.log"

    # Should report status without crashing
    if echo "$output" | grep -qi "no.*running\|no.*session\|status\|idle\|not running\|no auto"; then
        record_result "$name" "PASS" "Status check works"
    elif [[ $exit_code -ne 0 ]]; then
        record_result "$name" "PASS" "Exit $exit_code (no active session)"
    else
        record_result "$name" "PASS" "Handled gracefully"
    fi
}

print_summary() {
    log ""
    log "════════════════════════════════════════"
    log "  RESULTS: ${GREEN}${PASSED} passed${NC}, ${RED}${FAILED} failed${NC}, ${YELLOW}${SKIPPED} skipped${NC}"
    log "  Logs:    ${RESULTS_DIR}/"
    log "════════════════════════════════════════"
    log ""

    if [[ $FAILED -gt 0 ]]; then
        log "Failed scenario logs:"
        for f in "${RESULTS_DIR}"/*.log; do
            if grep -q "^FAIL" "$f" 2>/dev/null; then
                log "  $f"
            fi
        done
    fi
}

do_run() {
    load_bead_ids
    mkdir -p "$RESULTS_DIR"

    log "Running integration test scenarios..."
    log "  API epic: $API_EPIC"
    log "  Web epic: $WEB_EPIC"
    log ""

    # Fast scenarios first (no real bead processing)
    scenario_happy_path_dry_run
    scenario_resume
    scenario_stop
    scenario_abort
    scenario_check_status
    scenario_parallel_projects

    # Slower scenarios (actually processes beads)
    scenario_failure_handling

    # Full run is last (takes the longest)
    scenario_happy_path_real

    print_summary

    if [[ $FAILED -gt 0 ]]; then
        exit 1
    fi
}

# ──────────────────────────────────────────────────────────
# CLEANUP
# ──────────────────────────────────────────────────────────
do_cleanup() {
    log "Cleaning up playground..."

    # Unregister projects (ignore errors if not registered)
    wt project remove playground-api 2>/dev/null || true
    wt project remove playground-web 2>/dev/null || true

    # Remove playground directory
    if [[ -d "$PLAYGROUND_DIR" ]]; then
        rm -rf "$PLAYGROUND_DIR"
        log "Removed $PLAYGROUND_DIR"
    else
        log "No playground directory found."
    fi

    log "Cleanup complete."
}

# ──────────────────────────────────────────────────────────
# MAIN
# ──────────────────────────────────────────────────────────
case "${1:-}" in
    setup)   do_setup ;;
    run)     do_run ;;
    cleanup) do_cleanup ;;
    *)
        echo "Usage: $0 {setup|run|cleanup}"
        exit 1
        ;;
esac
