package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"chat-ai-whatsapp/internal/logger"
)

// OpenAI-compatible request/response types for 9Router

// ContentPart is a part of a multimodal message (text or image)
type ContentPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *ImageURLPart `json:"image_url,omitempty"`
}

type ImageURLPart struct {
	URL string `json:"url"`
}

// Message represents a chat message. Content can be:
// - string (for text-only messages)
// - []ContentPart (for multimodal messages with images)
type Message struct {
	Role       string      `json:"role"`
	Content    any         `json:"content"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type ToolDefinition struct {
	Type     string       `json:"type"`
	Function FunctionDef  `json:"function"`
}

type FunctionDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type chatRequest struct {
	Model       string          `json:"model"`
	Messages    []Message       `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	ToolChoice  string          `json:"tool_choice,omitempty"`
}

type chatResponse struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []choice      `json:"choices"`
	Usage   *usage        `json:"usage,omitempty"`
}

type choice struct {
	Index   int     `json:"index"`
	Message respMsg `json:"message"`
}

type respMsg struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function ToolFunction   `json:"function"`
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// 9Router returns a JSON object followed by "data: [DONE]"
// We strip the suffix and parse the JSON
type Client struct {
	baseURL  string
	model    string
	apiKey   string
	http     *http.Client
}

func New(baseURL, model, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/") + "/chat/completions",
		model:   model,
		apiKey:  apiKey,
		http: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Client) Chat(ctx context.Context, messages []Message) (string, []ToolCall, error) {
	return c.chat(ctx, messages, true)
}

func (c *Client) chat(ctx context.Context, messages []Message, includeTools bool) (string, []ToolCall, error) {
	req := chatRequest{
		Model:    c.model,
		Messages: messages,
	}

	if includeTools {
		req.Tools = []ToolDefinition{
			{
				Type: "function",
				Function: FunctionDef{
					Name:        "search_web",
					Description: "Cari informasi terbaru dari internet untuk menjawab pertanyaan sekolah, PR, atau pengetahuan umum",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{
								"type":        "string",
								"description": "Kata kunci pencarian",
							},
						},
						"required": []string{"query"},
					},
				},
			},
		}
		req.ToolChoice = "auto"
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return "", nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Set Host header biar 9Router anggap ini request local
	httpReq.Host = "localhost:20128"

	// Debug: log API key (masked) dan header
	keyMask := ""
	if len(c.apiKey) > 8 {
		keyMask = c.apiKey[:4] + "..." + c.apiKey[len(c.apiKey)-4:]
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	logger.Debug("9Router request: URL=%s, Key=%s, Auth=%s", c.baseURL, keyMask, httpReq.Header.Get("Authorization"))

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read body: %w", err)
	}

	logger.Debug("9Router response: status=%d, body=%s", resp.StatusCode, string(respBody[:min(len(respBody), 300)]))

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("9Router status %d: %s", resp.StatusCode, string(respBody))
	}

	// Strip trailing "data: [DONE]" if present (9Router appends it)
	cleaned := strings.TrimSpace(string(respBody))
	cleaned = strings.TrimSuffix(cleaned, "data: [DONE]")
	cleaned = strings.TrimSpace(cleaned)

	var chatResp chatResponse
	if err := json.Unmarshal([]byte(cleaned), &chatResp); err != nil {
		return "", nil, fmt.Errorf("parse response (len=%d): %w\nBody: %s", len(cleaned), err, cleaned[:min(len(cleaned), 200)])
	}

	if len(chatResp.Choices) == 0 {
		return "", nil, fmt.Errorf("no choices in response")
	}

	msg := chatResp.Choices[0].Message
	return msg.Content, msg.ToolCalls, nil
}


