package auto

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/badri/wt/internal/agentmail"
)

func TestNewAgentMailOrchestratorDisabledWhenUnavailable(t *testing.T) {
	amo := NewAgentMailOrchestrator("test")
	// Server not running, should be disabled
	if amo.IsEnabled() {
		t.Fatal("expected disabled when server unreachable")
	}
}

func TestDisabledOrchestratorNoOps(t *testing.T) {
	amo := &AgentMailOrchestrator{enabled: false}

	if err := amo.RegisterOrchestrator(); err != nil {
		t.Fatalf("RegisterOrchestrator should no-op: %v", err)
	}
	if err := amo.RegisterWorker("w1"); err != nil {
		t.Fatalf("RegisterWorker should no-op: %v", err)
	}
	id, err := amo.SendTask("w1", "bd-1", "prompt")
	if err != nil || id != "" {
		t.Fatalf("SendTask should no-op: id=%s err=%v", id, err)
	}
	beadID, status, body, err := amo.PollForCompletion()
	if err != nil || beadID != "" || status != "" || body != "" {
		t.Fatalf("PollForCompletion should no-op")
	}
}

func TestPollForCompletion(t *testing.T) {
	type mcpReq struct {
		Method string `json:"method"`
		Params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"params"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(200)
			return
		}
		var req mcpReq
		json.NewDecoder(r.Body).Decode(&req)

		switch req.Params.Name {
		case "fetch_inbox":
			msgs := []map[string]interface{}{
				{
					"message_id":   "m1",
					"from_agent":   "toast",
					"subject":      "DONE: bd-42",
					"body":         "implemented feature X",
					"acknowledged": false,
				},
			}
			text, _ := json.Marshal(msgs)
			resp := map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": string(text)},
				},
			}
			json.NewEncoder(w).Encode(resp)
		case "acknowledge_message":
			resp := map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": `{"status":"ok"}`},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	amo := &AgentMailOrchestrator{
		client:  newTestClient(server.URL, "test"),
		enabled: true,
	}

	beadID, status, body, err := amo.PollForCompletion()
	if err != nil {
		t.Fatalf("PollForCompletion: %v", err)
	}
	if beadID != "bd-42" {
		t.Errorf("expected bd-42, got %s", beadID)
	}
	if status != "done" {
		t.Errorf("expected done, got %s", status)
	}
	if body != "implemented feature X" {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestReconcileState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(200)
			return
		}

		type mcpReq struct {
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		var req mcpReq
		json.NewDecoder(r.Body).Decode(&req)

		switch req.Params.Name {
		case "fetch_inbox":
			msgs := []map[string]interface{}{
				{"message_id": "m1", "from_agent": "w1", "subject": "DONE: bd-2", "body": "done", "acknowledged": false},
				{"message_id": "m2", "from_agent": "w2", "subject": "DONE: bd-3", "body": "done", "acknowledged": false},
				{"message_id": "m3", "from_agent": "w1", "subject": "DONE: bd-99", "body": "done", "acknowledged": false}, // not in epic
			}
			text, _ := json.Marshal(msgs)
			resp := map[string]interface{}{
				"content": []map[string]interface{}{{"type": "text", "text": string(text)}},
			}
			json.NewEncoder(w).Encode(resp)
		case "acknowledge_message":
			resp := map[string]interface{}{
				"content": []map[string]interface{}{{"type": "text", "text": `{"status":"ok"}`}},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	amo := &AgentMailOrchestrator{
		client:  newTestClient(server.URL, "test"),
		enabled: true,
	}

	state := &EpicState{
		Beads:          []string{"bd-1", "bd-2", "bd-3"},
		CompletedBeads: []string{"bd-1"}, // bd-1 already known
	}

	missed, err := amo.ReconcileState(state)
	if err != nil {
		t.Fatalf("ReconcileState: %v", err)
	}
	if len(missed) != 2 {
		t.Fatalf("expected 2 missed, got %d", len(missed))
	}
	if missed[0] != "bd-2" || missed[1] != "bd-3" {
		t.Errorf("unexpected missed: %v", missed)
	}
}

func TestBuildWorkerInstructions(t *testing.T) {
	instructions := BuildWorkerInstructions("toast", "bd-42", "")
	if !strings.Contains(instructions, `"toast"`) {
		t.Error("should contain worker name")
	}
	if !strings.Contains(instructions, "DONE: bd-42") {
		t.Error("should contain DONE subject")
	}
	if !strings.Contains(instructions, "STUCK: bd-42") {
		t.Error("should contain STUCK subject")
	}
}

func newTestClient(baseURL, projectKey string) *agentmail.Client {
	return agentmail.NewClient(baseURL, projectKey)
}
