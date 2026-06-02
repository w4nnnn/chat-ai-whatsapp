package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"chat-ai-whatsapp/internal/logger"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type searxngResult struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	URL     string `json:"url"`
}

type searxngResponse struct {
	Results []searxngResult `json:"results"`
}

func (c *Client) Search(ctx context.Context, query string) (string, error) {
	u, err := url.Parse(c.baseURL + "/search")
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}

	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("language", "id")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("searxng status %d", resp.StatusCode)
	}

	var data searxngResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	var results []string
	for i, r := range data.Results {
		if r.Content == "" {
			continue
		}
		if i >= 5 {
			break
		}
		content := r.Content
		if len(content) > 500 {
			content = content[:500]
		}
		results = append(results, fmt.Sprintf("**%s**\n%s\n_Sumber: %s_", r.Title, content, r.URL))
	}

	if len(results) == 0 {
		return "Tidak ada hasil pencarian yang relevan.", nil
	}

	logger.Info("SearXNG: %d results for %q", len(results), query)
	return strings.Join(results, "\n\n---\n\n"), nil
}
