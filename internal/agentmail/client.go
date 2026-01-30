// Package agentmail provides an HTTP client for the MCP Agent Mail server.
// It enables wt auto mode to use agent-based messaging for reliable orchestration.
package agentmail

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	DefaultBaseURL = "http://127.0.0.1:8765"
	DefaultTimeout = 10 * time.Second
)

// Client communicates with an MCP Agent Mail server.
type Client struct {
	BaseURL    string
	ProjectKey string
	httpClient *http.Client
}

// NewClient creates a new Agent Mail client.
func NewClient(baseURL, projectKey string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL:    baseURL,
		ProjectKey: projectKey,
		httpClient: &http.Client{Timeout: DefaultTimeout},
	}
}

// Agent represents a registered agent identity.
type Agent struct {
	Name    string `json:"name"`
	Program string `json:"program,omitempty"`
	Model   string `json:"model,omitempty"`
}

// Message represents a mail message between agents.
type Message struct {
	ID           string    `json:"message_id"`
	From         string    `json:"from_agent"`
	To           []string  `json:"to_agents,omitempty"`
	Subject      string    `json:"subject"`
	Body         string    `json:"body"`
	AckRequired  bool      `json:"ack_required"`
	Acknowledged bool      `json:"acknowledged"`
	CreatedAt    time.Time `json:"created_at"`
}

// FileReservation represents an advisory file lock.
type FileReservation struct {
	AgentName string   `json:"agent_name"`
	Paths     []string `json:"paths"`
	Exclusive bool     `json:"exclusive"`
	TTL       int      `json:"ttl_seconds"`
}

// mcpRequest wraps a tool call in MCP format.
type mcpRequest struct {
	Method string    `json:"method"`
	Params mcpParams `json:"params"`
}

type mcpParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// mcpResponse is the envelope for MCP tool responses.
type mcpResponse struct {
	Content []mcpContent `json:"content,omitempty"`
	Error   *mcpError    `json:"error,omitempty"`
}

type mcpContent struct {
	Type string          `json:"type"`
	Text string          `json:"text,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *Client) callTool(toolName string, args interface{}) (json.RawMessage, error) {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal args: %w", err)
	}

	req := mcpRequest{
		Method: "tools/call",
		Params: mcpParams{
			Name:      toolName,
			Arguments: argsJSON,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(c.BaseURL+"/mcp/", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
	}

	var mcpResp mcpResponse
	if err := json.Unmarshal(respBody, &mcpResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if mcpResp.Error != nil {
		return nil, fmt.Errorf("mcp error %d: %s", mcpResp.Error.Code, mcpResp.Error.Message)
	}

	// Return the text content from first content block
	if len(mcpResp.Content) > 0 {
		if mcpResp.Content[0].Text != "" {
			return json.RawMessage(mcpResp.Content[0].Text), nil
		}
		if mcpResp.Content[0].Data != nil {
			return mcpResp.Content[0].Data, nil
		}
	}

	return nil, nil
}

// RegisterAgent creates an agent identity.
func (c *Client) RegisterAgent(name, program, model string) error {
	args := map[string]interface{}{
		"project_key": c.ProjectKey,
		"agent_name":  name,
		"program":     program,
		"model":       model,
	}
	_, err := c.callTool("register_agent", args)
	return err
}

// SendMessage sends a message from one agent to others.
func (c *Client) SendMessage(from string, to []string, subject, body string, ackRequired bool) (string, error) {
	args := map[string]interface{}{
		"project_key":  c.ProjectKey,
		"from_agent":   from,
		"to_agents":    to,
		"subject":      subject,
		"body":         body,
		"ack_required": ackRequired,
	}
	result, err := c.callTool("send_message", args)
	if err != nil {
		return "", err
	}

	var resp struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return "", fmt.Errorf("parse send_message response: %w", err)
	}
	return resp.MessageID, nil
}

// FetchInbox retrieves messages for an agent.
func (c *Client) FetchInbox(agentName string, limit int) ([]Message, error) {
	args := map[string]interface{}{
		"project_key": c.ProjectKey,
		"agent_name":  agentName,
		"limit":       limit,
	}
	result, err := c.callTool("fetch_inbox", args)
	if err != nil {
		return nil, err
	}

	var messages []Message
	if err := json.Unmarshal(result, &messages); err != nil {
		return nil, fmt.Errorf("parse inbox: %w", err)
	}
	return messages, nil
}

// AcknowledgeMessage marks a message as received.
func (c *Client) AcknowledgeMessage(agentName, messageID string) error {
	args := map[string]interface{}{
		"project_key": c.ProjectKey,
		"agent_name":  agentName,
		"message_id":  messageID,
	}
	_, err := c.callTool("acknowledge_message", args)
	return err
}

// ReserveFiles creates advisory file locks.
func (c *Client) ReserveFiles(agentName string, paths []string, ttlSeconds int, exclusive bool) error {
	args := map[string]interface{}{
		"project_key": c.ProjectKey,
		"agent_name":  agentName,
		"paths":       paths,
		"ttl_seconds": ttlSeconds,
		"exclusive":   exclusive,
	}
	_, err := c.callTool("file_reservation_paths", args)
	return err
}

// ReleaseFiles releases advisory file locks.
func (c *Client) ReleaseFiles(agentName string) error {
	args := map[string]interface{}{
		"project_key": c.ProjectKey,
		"agent_name":  agentName,
	}
	_, err := c.callTool("release_file_reservations", args)
	return err
}

// Ping checks if the Agent Mail server is reachable.
func (c *Client) Ping() error {
	resp, err := c.httpClient.Get(c.BaseURL + "/health")
	if err != nil {
		return fmt.Errorf("agent mail server unreachable: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent mail server returned %d", resp.StatusCode)
	}
	return nil
}

// IsAvailable returns true if the Agent Mail server is reachable.
func (c *Client) IsAvailable() bool {
	return c.Ping() == nil
}
