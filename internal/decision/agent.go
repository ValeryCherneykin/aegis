// Package decision defines the Agent contract: something that looks at
// category labels (never raw data) and returns one of a small, fixed set
// of actions. Two implementations ship out of the box — a MockAgent that
// needs no API key, and a ClaudeAgent that calls the real Anthropic API
// so you can see actual model reasoning bounded by the categorical gate.
package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// Decision is everything an Agent is allowed to hand back. Notice there is
// no field for amounts, names or IDs — the type itself makes leaking real
// data a compile error, not just a policy.
type Decision struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// Agent is the interface Aegis calls into. Swap MockAgent for ClaudeAgent,
// or write your own (OpenAI, a local model, whatever) — the rest of the
// system doesn't care.
type Agent interface {
	Decide(ctx context.Context, categories map[string]string, allowedActions []string) (Decision, error)
}

// ---------------------------------------------------------------------
// MockAgent: deterministic, zero-dependency stand-in for a real model.
// Good enough to demo the whole pipeline with nothing but `go run .`.
// ---------------------------------------------------------------------

// MockAgent is a deterministic, zero-dependency stand-in for a real AI model.
type MockAgent struct{}

// Decide implements the Agent interface using hardcoded rules for testing.
func (MockAgent) Decide(_ context.Context, categories map[string]string, _ []string) (Decision, error) {
	if categories["amount_tier"] == "HIGH" {
		return Decision{
			Action: "ESCALATE",
			Reason: fmt.Sprintf("amount_tier=HIGH exceeds auto-approval ceiling, despite return_history=%s", categories["return_history"]),
		}, nil
	}

	if categories["return_history"] == "FLAGGED" {
		return Decision{
			Action: "DENY",
			Reason: "return_history=FLAGGED suggests refund abuse pattern",
		}, nil
	}

	return Decision{
		Action: "ALLOW",
		Reason: "amount within auto-approval tier and clean return history",
	}, nil
}

// ---------------------------------------------------------------------
// ClaudeAgent: calls the real Anthropic API. Only category labels ever
// go into the prompt — grep this file, there is no amount/name/ID field
// anywhere near the HTTP request.
// ---------------------------------------------------------------------

// ClaudeAgent calls the real Anthropic API to make decisions based on categories.
type ClaudeAgent struct {
	APIKey string
	Model  string
	Client *http.Client
}

// NewClaudeAgentFromEnv builds a ClaudeAgent from ANTHROPIC_API_KEY. It
// returns (nil, false) if the key isn't set, so callers can cleanly fall
// back to MockAgent.
func NewClaudeAgentFromEnv() (*ClaudeAgent, bool) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, false
	}
	model := os.Getenv("AEGIS_MODEL")
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	return &ClaudeAgent{APIKey: key, Model: model, Client: &http.Client{Timeout: 20 * time.Second}}, true
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Decide implements the Agent interface by sending a prompt to the Anthropic API.
func (a *ClaudeAgent) Decide(ctx context.Context, categories map[string]string, allowedActions []string) (Decision, error) {
	prompt := buildPrompt(categories, allowedActions)

	reqBody := anthropicRequest{
		Model:     a.Model,
		MaxTokens: 300,
		System: "You are a business-rule reasoning module. You receive ONLY abstract " +
			"category labels, never raw amounts, names or IDs — that data physically " +
			"does not exist in your context. Decide the correct action using the " +
			"categories and the stated business logic. Respond with ONLY a JSON object " +
			`of the shape {"action": "...", "reason": "..."} and nothing else.`,
		Messages: []anthropicMessage{{Role: "user", Content: prompt}},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return Decision{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return Decision{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", a.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.Client.Do(req)
	if err != nil {
		return Decision{}, fmt.Errorf("calling anthropic api: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Decision{}, err
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Decision{}, fmt.Errorf("decoding anthropic response: %w", err)
	}
	if parsed.Error != nil {
		return Decision{}, fmt.Errorf("anthropic api error: %s", parsed.Error.Message)
	}
	if len(parsed.Content) == 0 {
		return Decision{}, fmt.Errorf("empty response from anthropic api")
	}

	text := strings.TrimSpace(parsed.Content[0].Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")

	var d Decision
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &d); err != nil {
		return Decision{}, fmt.Errorf("model did not return valid decision JSON: %q", text)
	}
	return d, nil
}

func buildPrompt(categories map[string]string, allowedActions []string) string {
	keys := make([]string, 0, len(categories))
	for k := range categories {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("Context (abstract categories only):\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "- %s: %s\n", k, categories[k])
	}
	fmt.Fprintf(&b, "\nAllowed actions: %s\n", strings.Join(allowedActions, ", "))
	b.WriteString("\nWhich action applies, and why? Reply with the JSON object only.")
	return b.String()
}
