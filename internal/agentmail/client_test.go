package agentmail

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := NewClient(server.URL, "test-project")
	return client, server
}

func mcpSuccessResponse(data interface{}) []byte {
	text, _ := json.Marshal(data)
	resp := mcpResponse{
		Content: []mcpContent{{Type: "text", Text: string(text)}},
	}
	b, _ := json.Marshal(resp)
	return b
}

func mcpErrorResponse(code int, msg string) []byte {
	resp := mcpResponse{
		Error: &mcpError{Code: code, Message: msg},
	}
	b, _ := json.Marshal(resp)
	return b
}

func TestRegisterAgent(t *testing.T) {
	var receivedArgs map[string]interface{}
	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req mcpRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.Unmarshal(req.Params.Arguments, &receivedArgs)
		w.Write(mcpSuccessResponse(map[string]string{"status": "ok"}))
	})

	err := client.RegisterAgent("orchestrator", "wt-auto", "claude-opus-4-5-20251101")
	if err != nil {
		t.Fatalf("RegisterAgent: %v", err)
	}

	if receivedArgs["agent_name"] != "orchestrator" {
		t.Errorf("expected agent_name=orchestrator, got %v", receivedArgs["agent_name"])
	}
	if receivedArgs["project_key"] != "test-project" {
		t.Errorf("expected project_key=test-project, got %v", receivedArgs["project_key"])
	}
}

func TestSendMessage(t *testing.T) {
	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(mcpSuccessResponse(map[string]string{"message_id": "msg-123"}))
	})

	id, err := client.SendMessage("orchestrator", []string{"toast"}, "task", "do work", true)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if id != "msg-123" {
		t.Errorf("expected msg-123, got %s", id)
	}
}

func TestFetchInbox(t *testing.T) {
	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		msgs := []Message{
			{ID: "m1", From: "toast", Subject: "DONE: bd-1", Body: "finished"},
			{ID: "m2", From: "toast", Subject: "STUCK: bd-2", Body: "blocked on X"},
		}
		w.Write(mcpSuccessResponse(msgs))
	})

	msgs, err := client.FetchInbox("orchestrator", 10)
	if err != nil {
		t.Fatalf("FetchInbox: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Subject != "DONE: bd-1" {
		t.Errorf("expected subject DONE: bd-1, got %s", msgs[0].Subject)
	}
}

func TestAcknowledgeMessage(t *testing.T) {
	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(mcpSuccessResponse(map[string]string{"status": "acknowledged"}))
	})

	err := client.AcknowledgeMessage("orchestrator", "msg-123")
	if err != nil {
		t.Fatalf("AcknowledgeMessage: %v", err)
	}
}

func TestMCPError(t *testing.T) {
	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write(mcpErrorResponse(-32000, "agent not found"))
	})

	err := client.RegisterAgent("unknown", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "mcp error -32000: agent not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHTTPError(t *testing.T) {
	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	})

	err := client.RegisterAgent("test", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPing(t *testing.T) {
	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
	})

	if err := client.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestPingUnreachable(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", "test")
	if client.IsAvailable() {
		t.Fatal("expected unavailable")
	}
}

func TestReserveFiles(t *testing.T) {
	var receivedArgs map[string]interface{}
	client, _ := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req mcpRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.Unmarshal(req.Params.Arguments, &receivedArgs)
		w.Write(mcpSuccessResponse(map[string]string{"status": "reserved"}))
	})

	err := client.ReserveFiles("toast", []string{"src/**"}, 3600, true)
	if err != nil {
		t.Fatalf("ReserveFiles: %v", err)
	}
	if receivedArgs["agent_name"] != "toast" {
		t.Errorf("expected agent_name=toast, got %v", receivedArgs["agent_name"])
	}
}
