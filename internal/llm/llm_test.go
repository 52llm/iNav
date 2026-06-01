package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientTagParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/incorrect auth header: %q", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "existing-tag") {
			t.Errorf("existing tags not included in prompt: %s", body)
		}
		// Mimic an OpenAI chat completion whose message content is our JSON.
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{
					"content": `{"tags":["Go","web"],"summary":"A Go web framework."}`,
				}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-key", "gpt-test")
	out, err := c.Tag(context.Background(), TagInput{
		Title:        "Gin",
		URL:          "https://gin-gonic.com",
		Content:      "Gin is a web framework written in Go.",
		ExistingTags: []string{"existing-tag"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Summary != "A Go web framework." {
		t.Errorf("summary = %q", out.Summary)
	}
	if len(out.Tags) != 2 || out.Tags[0] != "Go" {
		t.Errorf("tags = %v", out.Tags)
	}
}

func TestClientTagErrorsOnMalformedContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "not json"}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, "k", "m")
	if _, err := c.Tag(context.Background(), TagInput{Title: "x"}); err == nil {
		t.Fatal("expected error on malformed JSON content")
	}
}
