package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TagInput is the page data the model uses to produce tags + a summary.
type TagInput struct {
	Title        string
	URL          string
	Excerpt      string
	Content      string
	ExistingTags []string
}

// TagResult is the model's structured output.
type TagResult struct {
	Tags    []string `json:"tags"`
	Summary string   `json:"summary"`
}

// Tagger produces tags + a summary for a captured page.
type Tagger interface {
	Tag(ctx context.Context, in TagInput) (TagResult, error)
}

// Client calls an OpenAI-compatible /chat/completions endpoint.
type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

// New builds a Client. baseURL is e.g. "https://api.openai.com/v1".
func New(baseURL, apiKey, model string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) Tag(ctx context.Context, in TagInput) (TagResult, error) {
	content := strings.TrimSpace(in.Content)
	if len(content) > 6000 {
		content = content[:6000]
	}
	prompt := fmt.Sprintf(`You assign browsing tags to a web page for a personal bookmark portal,
where tags are the buckets a person filters by in a sidebar. Good tags are
BROAD and REUSABLE across many pages; avoid narrow, page-specific ones.

Output ONLY a JSON object: {"tags": ["..."], "summary": "one concise sentence"}.

Tag rules:
- Each tag MUST be a NOUN naming a subject, domain, technology, tool, or field
  — what the page is ABOUT. NEVER use adjectives or attributes that merely
  describe the page.
  Good (noun topics): "python", "go", "databases", "web", "ai", "security", "networking", "kubernetes".
  Bad (adjectives/attributes — never use): "open-source", "self-hosted", "lightweight", "free", "fast", "modern", "cross-platform".
- Reuse an EXISTING tag whenever the page reasonably fits it. Strongly prefer
  reusing over inventing a new tag.
- Invent a new tag only if no existing tag fits, and make it broad enough to
  recur on future pages.
- Prefer broad topics over narrow ones; avoid one-off specifics like
  "http-router", "api-wrapper", "orm".
- When unsure between a general and a specific tag, choose the general one.
- Tags are lowercase ENGLISH. Use 1-3 tags.
- summary: one neutral sentence describing what the page is.

EXISTING TAGS (reuse these first): %s

PAGE TITLE: %s
PAGE URL: %s
PAGE EXCERPT: %s
PAGE CONTENT:
%s`,
		strings.Join(in.ExistingTags, ", "), in.Title, in.URL, in.Excerpt, content)

	reqBody := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"temperature":     0,
	}
	buf, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return TagResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return TagResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return TagResult{}, fmt.Errorf("llm http %d", resp.StatusCode)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return TagResult{}, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return TagResult{}, fmt.Errorf("llm returned no choices")
	}

	var out TagResult
	if err := json.Unmarshal([]byte(parsed.Choices[0].Message.Content), &out); err != nil {
		return TagResult{}, fmt.Errorf("parse model content as json: %w", err)
	}
	if len(out.Tags) == 0 {
		return TagResult{}, fmt.Errorf("model returned no tags")
	}
	return out, nil
}
